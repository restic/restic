package dump

import (
	"context"
	"io"
	"path"
	"sync"

	"golang.org/x/sync/semaphore"

	"github.com/restic/restic/internal/bloblru"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

// A Dumper writes trees and files from a repository to a Writer
// in an archive format.
type Dumper struct {
	cache  *bloblru.Cache
	format string
	repo   restic.Repository
	w      io.Writer
}

func New(format string, repo restic.Repository, w io.Writer) *Dumper {
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

func sendTrees(ctx context.Context, repo restic.Repository, tree *restic.Tree, rootPath string, ch chan *restic.Node) {
	defer close(ch)

	for _, root := range tree.Nodes {
		root.Path = path.Join(rootPath, root.Name)
		if sendNodes(ctx, repo, root, ch) != nil {
			break
		}
	}
}

func sendNodes(ctx context.Context, repo restic.Repository, root *restic.Node, ch chan *restic.Node) error {
	select {
	case ch <- root:
	case <-ctx.Done():
		return ctx.Err()
	}

	// If this is no directory we are finished
	if !IsDir(root) {
		return nil
	}

	err := walker.Walk(ctx, repo, *root.Subtree, nil, func(_ restic.ID, nodepath string, node *restic.Node, err error) (bool, error) {
		if err != nil {
			return false, err
		}
		if node == nil {
			return false, nil
		}

		node.Path = path.Join(root.Path, nodepath)

		if !IsFile(node) && !IsDir(node) && !IsLink(node) {
			return false, nil
		}

		select {
		case ch <- node:
		case <-ctx.Done():
			return false, ctx.Err()
		}

		return false, nil
	})

	return err
}

/*
// WriteNode writes a file node's contents directly to d's Writer,
// without caring about d's format.
func (d *Dumper) WriteNode(ctx context.Context, node *restic.Node) error {
	return d.writeNode(ctx, d.w, node)
}
*/

func (d *Dumper) writeNode(ctx context.Context, w io.Writer, node *restic.Node) error {
	var (
		buf []byte
		err error
	)
	for _, id := range node.Content {
		blob, ok := d.cache.Get(id)
		if !ok {
			blob, err = d.repo.LoadBlob(ctx, restic.DataBlob, id, buf)
			if err != nil {
				return err
			}

			buf = d.cache.Add(id, blob) // Reuse evicted buffer.
		}

		if _, err := w.Write(blob); err != nil {
			return errors.Wrap(err, "Write")
		}
	}

	return nil
}

// WriteNode writes a file node's contents directly to d's Writer,
// maintaining order while loading blobs in parallel.
func (d *Dumper) WriteNode(ctx context.Context, node *restic.Node) error {
	// Using a semaphore to limit concurrent blob loads
	maxWorkers := 20 // Adjust the number of workers based on your environment
	sem := semaphore.NewWeighted(int64(maxWorkers))

	var wg sync.WaitGroup
	orderedBlobs := make([][]byte, len(node.Content))
	var firstError error
	var mu sync.Mutex // To synchronize error handling

	for i, id := range node.Content {
		wg.Add(1)
		go func(index int, blobID restic.ID) {
			defer wg.Done()

			// Acquire semaphore to limit concurrency
			if err := sem.Acquire(ctx, 1); err != nil {
				mu.Lock()
				if firstError == nil {
					firstError = err
				}
				mu.Unlock()
				return
			}
			defer sem.Release(1)

			blob, err := d.loadBlob(ctx, blobID)
			if err != nil {
				mu.Lock()
				if firstError == nil {
					firstError = err
				}
				mu.Unlock()
				return
			}
			orderedBlobs[index] = blob
		}(i, id)
	}

	wg.Wait()

	if firstError != nil {
		return firstError
	}

	// Write blobs in the original order
	for _, blob := range orderedBlobs {
		if _, err := d.w.Write(blob); err != nil {
			return errors.Wrap(err, "Write")
		}
	}

	return nil
}

// Load blob handling cache and repo
func (d *Dumper) loadBlob(ctx context.Context, id restic.ID) ([]byte, error) {
	buf, ok := d.cache.Get(id)
	if !ok {
		var err error
		buf, err = d.repo.LoadBlob(ctx, restic.DataBlob, id, nil)
		if err != nil {
			return nil, err
		}
		buf = d.cache.Add(id, buf)
	}
	return buf, nil
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
