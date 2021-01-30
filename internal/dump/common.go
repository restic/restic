package dump

import (
	"context"
	"io"
	"path"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

// dumper implements saving node data.
type dumper interface {
	io.Closer
	dumpNode(ctx context.Context, node *restic.Node, repo restic.Repository) error
}

// WriteDump will write the contents of the given tree to the given destination.
// It will loop over all nodes in the tree and dump them recursively.
type WriteDump func(ctx context.Context, repo restic.Repository, tree *restic.Tree, rootPath string, dst io.Writer) error

func writeDump(ctx context.Context, repo restic.Repository, tree *restic.Tree, rootPath string, dmp dumper, dst io.Writer) error {
	for _, rootNode := range tree.Nodes {
		rootNode.Path = rootPath
		err := dumpTree(ctx, repo, rootNode, rootPath, dmp)
		if err != nil {
			// ignore subsequent errors
			_ = dmp.Close()

			return err
		}
	}

	return dmp.Close()
}

func dumpTree(ctx context.Context, repo restic.Repository, rootNode *restic.Node, rootPath string, dmp dumper) error {
	rootNode.Path = path.Join(rootNode.Path, rootNode.Name)
	rootPath = rootNode.Path

	if err := dmp.dumpNode(ctx, rootNode, repo); err != nil {
		return err
	}

	// If this is no directory we are finished
	if !IsDir(rootNode) {
		return nil
	}

	err := walker.Walk(ctx, repo, *rootNode.Subtree, nil, func(_ restic.ID, nodepath string, node *restic.Node, err error) (bool, error) {
		if err != nil {
			return false, err
		}
		if node == nil {
			return false, nil
		}

		node.Path = path.Join(rootPath, nodepath)

		if IsFile(node) || IsLink(node) || IsDir(node) {
			err := dmp.dumpNode(ctx, node, repo)
			if err != nil {
				return false, err
			}
		}

		return false, nil
	})

	return err
}

// GetNodeData will write the contents of the node to the given output.
func GetNodeData(ctx context.Context, output io.Writer, repo restic.Repository, node *restic.Node) error {
	var (
		buf []byte
		err error
	)
	for _, id := range node.Content {
		buf, err = repo.LoadBlob(ctx, restic.DataBlob, id, buf)
		if err != nil {
			return err
		}

		_, err = output.Write(buf)
		if err != nil {
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
