package dump

import (
	"context"
	"io"
	"path"

	"github.com/restic/restic/internal/bloblru"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
	"golang.org/x/sync/errgroup"
)

// A Dumper writes trees and files from a repository to a Writer
// in an archive format.
type Dumper struct {
	cache  *bloblru.Cache
	format string
	repo   restic.Loader
	w      io.Writer
	// Error reports a node that is skipped due to an error. Currently only used to report out-of-order directory entries.
	Error func(nodePath string, err error)
}

func New(format string, repo restic.Loader, w io.Writer) *Dumper {
	return &Dumper{
		cache:  bloblru.New(64 << 20),
		format: format,
		repo:   repo,
		w:      w,
		Error:  func(string, error) {},
	}
}

func (d *Dumper) DumpTree(ctx context.Context, tree data.TreeNodeIterator, rootPath string) error {
	wg, ctx := errgroup.WithContext(ctx)

	// ch is buffered to deal with variable download/write speeds.
	ch := make(chan *data.Node, 10)
	wg.Go(func() error {
		return d.sendTrees(ctx, tree, rootPath, ch)
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

func (d *Dumper) sendTrees(ctx context.Context, nodes data.TreeNodeIterator, rootPath string, ch chan *data.Node) error {
	defer close(ch)

	for item := range data.EnforceTreeOrder(nodes) {
		if item.Error != nil {
			if errors.Is(item.Error, data.ErrTreeNotOrdered) {
				d.Error(path.Join(rootPath, item.Node.Name), item.Error)
				continue
			}
			return item.Error
		}
		node := item.Node
		node.Path = path.Join(rootPath, node.Name)
		if err := d.sendNodes(ctx, node, ch); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dumper) sendNodes(ctx context.Context, root *data.Node, ch chan *data.Node) error {
	select {
	case ch <- root:
	case <-ctx.Done():
		return ctx.Err()
	}

	// If this is no directory we are finished
	if root.Type != data.NodeTypeDir {
		return nil
	}

	return walker.Walk(ctx, d.repo, *root.Subtree, walker.WalkVisitor{
		EnforceTreeOrder: true,
		ProcessNode: func(_ restic.ID, nodePath string, node *data.Node, err error) error {
			if errors.Is(err, data.ErrTreeNotOrdered) {
				d.Error(path.Join(root.Path, nodePath), err)
				// Skip only this entry. ErrSkipNode would drop the rest of a non-directory.
				return nil
			} else if err != nil {
				return err
			} else if node == nil {
				return nil
			}

			node.Path = path.Join(root.Path, nodePath)
			if node.Type != data.NodeTypeFile && node.Type != data.NodeTypeDir && node.Type != data.NodeTypeSymlink {
				return nil
			}

			select {
			case ch <- node:
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		},
	})
}

// WriteNode writes a file node's contents directly to d's Writer,
// without caring about d's format.
func (d *Dumper) WriteNode(ctx context.Context, node *data.Node) error {
	return d.writeNode(ctx, d.w, node)
}

func (d *Dumper) writeNode(ctx context.Context, w io.Writer, node *data.Node) error {
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
				return d.repo.LoadBlob(ctx, restic.BlobHandle{Type: restic.DataBlob, ID: id}, nil)
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
