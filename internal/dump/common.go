package dump

import (
	"context"
	"io"
	"path"

	"github.com/restic/restic/internal/bloblru"
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
}

func New(format string, repo restic.Loader, w io.Writer) *Dumper {
	return &Dumper{
		cache:  bloblru.New(64 << 20),
		format: format,
		repo:   repo,
		w:      w,
	}
}

func (d *Dumper) DumpTree(ctx context.Context, tree *restic.Tree, rootPath string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// ch is buffered to deal with variable download/write speeds.
	ch := make(chan *restic.Node, 10)
	go sendTrees(ctx, d.repo, tree, rootPath, ch)

	switch d.format {
	case "tar":
		return d.dumpTar(ctx, ch)
	case "zip":
		return d.dumpZip(ctx, ch)
	default:
		panic("unknown dump format")
	}
}

func sendTrees(ctx context.Context, repo restic.BlobLoader, tree *restic.Tree, rootPath string, ch chan *restic.Node) {
	defer close(ch)

	for _, root := range tree.Nodes {
		root.Path = path.Join(rootPath, root.Name)
		if sendNodes(ctx, repo, root, ch) != nil {
			break
		}
	}
}

func sendNodes(ctx context.Context, repo restic.BlobLoader, root *restic.Node, ch chan *restic.Node) error {
	select {
	case ch <- root:
	case <-ctx.Done():
		return ctx.Err()
	}

	// If this is no directory we are finished
	if root.Type != restic.NodeTypeDir {
		return nil
	}

	err := walker.Walk(ctx, repo, *root.Subtree, walker.WalkVisitor{ProcessNode: func(_ restic.ID, nodepath string, node *restic.Node, err error) error {
		if err != nil {
			return err
		}
		if node == nil {
			return nil
		}

		node.Path = path.Join(root.Path, nodepath)

		if node.Type != restic.NodeTypeFile && node.Type != restic.NodeTypeDir && node.Type != restic.NodeTypeSymlink {
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
func (d *Dumper) WriteNode(ctx context.Context, node *restic.Node) error {
	return d.writeNode(ctx, d.w, node)
}

func (d *Dumper) writeNode(ctx context.Context, w io.Writer, node *restic.Node) error {
	wg, ctx := errgroup.WithContext(ctx)
	limit := d.repo.Connections() - 1 // See below for the -1.
	blobs := make(chan (<-chan []byte), limit)

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
	// There will be at most 1+cap(blobs) calling LoadBlob at any moment.
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
