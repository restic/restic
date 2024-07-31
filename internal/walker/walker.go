package walker

import (
	"context"
	"path"
	"sort"

	"github.com/pkg/errors"

	"github.com/restic/restic/internal/restic"
)

// ErrSkipNode is returned by WalkFunc when a dir node should not be walked.
var ErrSkipNode = errors.New("skip this node")

// WalkFunc is the type of the function called for each node visited by Walk.
// Path is the slash-separated path from the root node. If there was a problem
// loading a node, err is set to a non-nil error. WalkFunc can chose to ignore
// it by returning nil.
//
// When the special value ErrSkipNode is returned and node is a dir node, it is
// not walked. When the node is not a dir node, the remaining items in this
// tree are skipped.
type WalkFunc func(parentTreeID restic.ID, path string, node *restic.Node, nodeErr error) (err error)

type WalkVisitor struct {
	// If the node is a `dir`, it will be entered afterwards unless `ErrSkipNode`
	// was returned. This function is mandatory
	ProcessNode WalkFunc
	// Optional callback
	LeaveDir func(path string)
}

// Walk calls walkFn recursively for each node in root. If walkFn returns an
// error, it is passed up the call stack. The trees in ignoreTrees are not
// walked. If walkFn ignores trees, these are added to the set.
func Walk(ctx context.Context, repo restic.BlobLoader, root restic.ID, visitor WalkVisitor) error {
	tree, err := restic.LoadTree(ctx, repo, root)
	err = visitor.ProcessNode(root, "/", nil, err)

	if err != nil {
		if err == ErrSkipNode {
			err = nil
		}
		return err
	}

	return walk(ctx, repo, "/", root, tree, visitor)
}

// walk recursively traverses the tree, ignoring subtrees when the ID of the
// subtree is in ignoreTrees. If err is nil and ignore is true, the subtree ID
// will be added to ignoreTrees by walk.
func walk(ctx context.Context, repo restic.BlobLoader, prefix string, parentTreeID restic.ID, tree *restic.Tree, visitor WalkVisitor) (err error) {
	sort.Slice(tree.Nodes, func(i, j int) bool {
		return tree.Nodes[i].Name < tree.Nodes[j].Name
	})

	for _, node := range tree.Nodes {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		p := path.Join(prefix, node.Name)

		if node.Type == "" {
			return errors.Errorf("node type is empty for node %q", node.Name)
		}

		if node.Type != "dir" {
			err := visitor.ProcessNode(parentTreeID, p, node, nil)
			if err != nil {
				if err == ErrSkipNode {
					// skip the remaining entries in this tree
					break
				}

				return err
			}

			continue
		}

		if node.Subtree == nil {
			return errors.Errorf("subtree for node %v in tree %v is nil", node.Name, p)
		}

		subtree, err := restic.LoadTree(ctx, repo, *node.Subtree)
		err = visitor.ProcessNode(parentTreeID, p, node, err)
		if err != nil {
			if err == ErrSkipNode {
				continue
			}
		}

		err = walk(ctx, repo, p, *node.Subtree, subtree, visitor)
		if err != nil {
			return err
		}
	}

	if visitor.LeaveDir != nil {
		visitor.LeaveDir(prefix)
	}

	return nil
}
