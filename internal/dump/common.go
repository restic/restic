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
	bufferPool sync.Pool
}

func New(format string, repo restic.Repository, w io.Writer) *Dumper {
	return &Dumper{
		cache:  bloblru.New(64 << 20),
		format: format,
		repo:   repo,
		w:      w,
		bufferPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 1024 * 1024)  // 1MB buffer
			},
		},
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

func (d *Dumper) WriteNode(ctx context.Context, node *restic.Node) error {
	batchSize := 20 // Define a suitable batch size
	batchChan := make(chan []restic.ID, 20) // Buffered channel for batches

	// Start a goroutine to enqueue batches and close the channel once done
	go func() {
		d.enqueueBatches(ctx, []*restic.Node{node}, batchChan, batchSize)
		close(batchChan) // Close the batch channel after all batches are sent
	}()

	sem := semaphore.NewWeighted(100) // Semaphore to control the number of concurrent goroutines
	var wg sync.WaitGroup
	errChan := make(chan error, 1) // Channel for error reporting

	worker := func() {
		defer wg.Done()
		for batch := range batchChan {
			if err := sem.Acquire(ctx, 1); err != nil {
				select {
				case errChan <- err:
				default:
				}
				return
			}

			// Process each ID in the batch
			for _, id := range batch {
				buf := d.bufferPool.Get().([]byte)
				blob, err := d.repo.LoadBlob(ctx, restic.DataBlob, id, buf)
				d.bufferPool.Put(buf)
				if err != nil {
					sem.Release(1)
					select {
					case errChan <- err:
					default:
					}
					return
				}

				if _, err := d.w.Write(blob); err != nil {
					sem.Release(1)
					select {
					case errChan <- err:
					default:
					}
					return
				}
			}
			sem.Release(1)
		}
	}

	// Start workers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go worker()
	}

	wg.Wait() // Wait for all workers to finish
	close(errChan) // Close the error channel after all workers are done

	// Check for errors
	if err, ok := <-errChan; ok {
		return err
	}

	return nil
}

func (d *Dumper) enqueueBatches(ctx context.Context, nodes []*restic.Node, batchChan chan<- []restic.ID, batchSize int) {
	var batch []restic.ID
	for _, node := range nodes {
		for _, id := range node.Content {
			batch = append(batch, id)
			if len(batch) == batchSize {
				batchChan <- batch
				batch = nil // Start a new batch
			}
		}
	}
	if len(batch) > 0 { // Send any remaining blobs in the last batch
		batchChan <- batch
	}
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
