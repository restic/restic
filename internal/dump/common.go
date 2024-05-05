package dump

import (
	"context"
	"io"
	"path"

	"github.com/restic/restic/internal/bloblru"
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
	if !IsDir(root) {
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

		if !IsFile(node) && !IsDir(node) && !IsLink(node) {
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
	type loadTask struct {
		id  restic.ID
		out chan<- []byte
	}
	type writeTask struct {
		data <-chan []byte
	}

	loaderCh := make(chan loadTask)
	// per worker: allows for one blob that gets download + one blob thats queue for writing
	writerCh := make(chan writeTask, d.repo.Connections()*2)

	wg, ctx := errgroup.WithContext(ctx)

	wg.Go(func() error {
		defer close(loaderCh)
		defer close(writerCh)
		for _, id := range node.Content {
			// non-blocking blob handover to allow the loader to load the next blob
			// while the old one is still written
			ch := make(chan []byte, 1)
			select {
			case loaderCh <- loadTask{id: id, out: ch}:
			case <-ctx.Done():
				return ctx.Err()
			}

			select {
			case writerCh <- writeTask{data: ch}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	for i := uint(0); i < d.repo.Connections(); i++ {
		wg.Go(func() error {
			for task := range loaderCh {
				blob, err := d.cache.GetOrCompute(task.id, func() ([]byte, error) {
					return d.repo.LoadBlob(ctx, restic.DataBlob, task.id, nil)
				})
				if err != nil {
					return err
				}

				select {
				case task.out <- blob:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
	}

	wg.Go(func() error {
		for result := range writerCh {
			select {
			case data := <-result.data:
				if _, err := w.Write(data); err != nil {
					return errors.Wrap(err, "Write")
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	return wg.Wait()
}

// IsDir checks if the given node is a directory.
func IsDir(node *restic.Node) bool {
	return node.Type == "dir"
}

// IsLink checks if the given node as a link.
func IsLink(node *restic.Node) bool {
	return node.Type == "symlink"
}

// IsFile checks if the given node is a file.
func IsFile(node *restic.Node) bool {
	return node.Type == "file"
}
