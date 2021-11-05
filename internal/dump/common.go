package dump

import (
	"context"
	"io"
	"path"

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

// WriteNode writes a file node's contents directly to d's Writer,
// without caring about d's format.
func (d *Dumper) WriteNode(ctx context.Context, node *restic.Node) error {
	return d.writeNode(ctx, d.w, node)
}

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
