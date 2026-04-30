package dump

import (
	"context"
	"fmt"
	"io"
	"path"

	"github.com/restic/restic/internal/bloblru"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
	"golang.org/x/sync/errgroup"
)

// A Dumper writes trees and files from a repository to a Writer
// in an archive format.
type Dumper interface {
	WriteNode(ctx context.Context, node *data.Node) error
	DumpTree(ctx context.Context, tree data.TreeNodeIterator, rootPath string) error
}

// SequentialDumper writes trees and files sequentially.
type SequentialDumper struct {
	cache  *bloblru.Cache
	format string
	repo   restic.Loader
	writer io.Writer
}

type ParallelDumper struct {
	seq       *SequentialDumper
	writerAt  io.WriterAt
	skipZeros bool
}

func NewSequentialDumper(format string, repo restic.Loader, writer io.Writer) *SequentialDumper {
	return &SequentialDumper{
		cache:  bloblru.New(64 << 20),
		format: format,
		repo:   repo,
		writer: writer,
	}
}

func NewParallelDumper(seq *SequentialDumper, writerAt io.WriterAt, skipZeros bool) *ParallelDumper {
	return &ParallelDumper{
		seq:       seq,
		writerAt:  writerAt,
		skipZeros: skipZeros,
	}
}

func (d *SequentialDumper) DumpTree(ctx context.Context, tree data.TreeNodeIterator, rootPath string) error {
	wg, ctx := errgroup.WithContext(ctx)

	// ch is buffered to deal with variable download/write speeds.
	ch := make(chan *data.Node, 10)
	wg.Go(func() error {
		return sendTrees(ctx, d.repo, tree, rootPath, ch)
	})

	wg.Go(func() error {
		switch d.format {
		case "tar":
			return d.dumpTar(ctx, ch)
		case "zip":
			return d.dumpZip(ctx, ch)
		default:
			panic("unknown dump format")
		}
	})
	return wg.Wait()
}

func sendTrees(ctx context.Context, repo restic.BlobLoader, nodes data.TreeNodeIterator, rootPath string, ch chan *data.Node) error {
	defer close(ch)

	for item := range nodes {
		if item.Error != nil {
			return item.Error
		}
		node := item.Node
		node.Path = path.Join(rootPath, node.Name)
		if err := sendNodes(ctx, repo, node, ch); err != nil {
			return err
		}
	}
	return nil
}

func sendNodes(ctx context.Context, repo restic.BlobLoader, root *data.Node, ch chan *data.Node) error {
	select {
	case ch <- root:
	case <-ctx.Done():
		return ctx.Err()
	}

	// If this is no directory we are finished
	if root.Type != data.NodeTypeDir {
		return nil
	}

	err := walker.Walk(ctx, repo, *root.Subtree, walker.WalkVisitor{ProcessNode: func(_ restic.ID, nodepath string, node *data.Node, err error) error {
		if err != nil {
			return err
		}
		if node == nil {
			return nil
		}

		node.Path = path.Join(root.Path, nodepath)

		if node.Type != data.NodeTypeFile && node.Type != data.NodeTypeDir && node.Type != data.NodeTypeSymlink {
			return nil
		}

		select {
		case ch <- node:
		case <-ctx.Done():
			return ctx.Err()
		}

		return nil
	}})

	return err
}

// WriteNode writes a file node's contents directly to d's Writer,
// without caring about d's format.
func (d *SequentialDumper) WriteNode(ctx context.Context, node *data.Node) error {
	return d.writeNode(ctx, d.writer, node)
}

func (d *SequentialDumper) writeNode(ctx context.Context, w io.Writer, node *data.Node) error {
	wg, ctx := errgroup.WithContext(ctx)
	limit := int(d.repo.Connections())
	wg.SetLimit(1 + limit) // +1 for the writer.
	blobs := make(chan (<-chan []byte), limit)

	// Writer.
	wg.Go(func() error {
		for ch := range blobs {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case blob := <-ch:
				if _, err := w.Write(blob); err != nil {
					return err
				}
			}
		}
		return nil
	})

	// Start short-lived goroutines to load blobs.
loop:
	for _, id := range node.Content {
		// This needs to be buffered, so that loaders can quit
		// without waiting for the writer.
		ch := make(chan []byte, 1)

		wg.Go(func() error {
			blob, err := d.cache.GetOrCompute(id, func() ([]byte, error) {
				return d.repo.LoadBlob(ctx, restic.DataBlob, id, nil)
			})

			if err == nil {
				ch <- blob
			}
			return err
		})

		select {
		case blobs <- ch:
		case <-ctx.Done():
			break loop
		}
	}

	close(blobs)
	return wg.Wait()
}

func (p *ParallelDumper) WriteNode(ctx context.Context, node *data.Node) error {
	return p.writeNode(ctx, p.writerAt, node)
}

func (p *ParallelDumper) DumpTree(ctx context.Context, tree data.TreeNodeIterator, rootPath string) error {
	return p.seq.DumpTree(ctx, tree, rootPath)
}

type BlobTask struct {
	id     restic.ID
	offset int64
}

func (p *ParallelDumper) writeNode(ctx context.Context, w io.WriterAt, node *data.Node) error {
	wg, ctx := errgroup.WithContext(ctx)
	limit := int(p.seq.repo.Connections())
	wg.SetLimit(limit)

	taskChan := make(chan BlobTask, limit*2)

	for i := 0; i < limit; i++ {
		wg.Go(func() error {
			for task := range taskChan {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					blob, err := p.seq.cache.GetOrCompute(task.id, func() ([]byte, error) {
						return p.seq.repo.LoadBlob(ctx, restic.DataBlob, task.id, nil)
					})
					if err != nil {
						return err
					}
					if _, err := w.WriteAt(blob, task.offset); err != nil {
						return err
					}
				}
			}
			return nil
		})
	}

	var currentOffset int64 = 0
	for _, id := range node.Content {
		size, found := p.seq.repo.LookupBlobSize(restic.DataBlob, id)
		if !found {
			return fmt.Errorf("blob %v not found", id)
		}
		if p.skipZeros && (id == repository.ZeroChunk()) {
			currentOffset += int64(size)
			continue
		}

		select {
		case taskChan <- BlobTask{
			id:     id,
			offset: currentOffset,
		}:
		case <-ctx.Done():
			return ctx.Err()
		}

		currentOffset += int64(size)
	}
	close(taskChan)

	return wg.Wait()
}
