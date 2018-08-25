package walker

import (
	"context"
	"path"
	"sort"

	"github.com/pkg/errors"

	"github.com/restic/restic/internal/restic"
)

// TreeLoader loads a tree from a repository.
type TreeLoader interface {
	LoadTree(context.Context, restic.ID) (*restic.Tree, error)
}

// SkipNode is returned by WalkFunc when a dir node should not be walked.
var SkipNode = errors.New("skip this node")

// WalkFunc is the type of the function called for each node visited by Walk.
// Path is the slash-separated path from the root node. If there was a problem
// loading a node, err is set to a non-nil error. WalkFunc can chose to ignore
// it by returning nil.
//
// When the special value SkipNode is returned and node is a dir node, it is
// not walked. When the node is not a dir node, the remaining items in this
// tree are skipped.
//
// Setting ignore to true tells Walk that it should not visit the node again.
// For tree nodes, this means that the function is not called for the
// referenced tree. If the node is not a tree, and all nodes in the current
// tree have ignore set to true, the current tree will not be visited again.
// When err is not nil and different from SkipNode, the value returned for
// ignore is ignored.
type WalkFunc func(parentTreeID restic.ID, path string, node *restic.Node, nodeErr error) (ignore bool, err error)

// Walk calls walkFn recursively for each node in root. If walkFn returns an
// error, it is passed up the call stack. The trees in ignoreTrees are not
// walked. If walkFn ignores trees, these are added to the set.
func Walk(ctx context.Context, repo TreeLoader, root restic.ID, ignoreTrees restic.IDSet, walkFn WalkFunc) error {
	tree, err := repo.LoadTree(ctx, root)
	_, err = walkFn(root, "/", nil, err)

	if err != nil {
		if err == SkipNode {
			err = nil
		}
		return err
	}

	if ignoreTrees == nil {
		ignoreTrees = restic.NewIDSet()
	}

	_, err = walk(ctx, repo, "/", root, tree, ignoreTrees, walkFn)
	return err
}

// walk recursively traverses the tree, ignoring subtrees when the ID of the
// subtree is in ignoreTrees. If err is nil and ignore is true, the subtree ID
// will be added to ignoreTrees by walk.
func walk(ctx context.Context, repo TreeLoader, prefix string, parentTreeID restic.ID, tree *restic.Tree, ignoreTrees restic.IDSet, walkFn WalkFunc) (ignore bool, err error) {
	var allNodesIgnored = true

	if len(tree.Nodes) == 0 {
		allNodesIgnored = false
	}

	sort.Slice(tree.Nodes, func(i, j int) bool {
		return tree.Nodes[i].Name < tree.Nodes[j].Name
	})

	for _, node := range tree.Nodes {
		p := path.Join(prefix, node.Name)

		if node.Type == "" {
			return false, errors.Errorf("node type is empty for node %q", node.Name)
		}

		if node.Type != "dir" {
			ignore, err := walkFn(parentTreeID, p, node, nil)
			if err != nil {
				if err == SkipNode {
					// skip the remaining entries in this tree
					return allNodesIgnored, nil
				}

				return false, err
			}

			if ignore == false {
				allNodesIgnored = false
			}

			continue
		}

		if node.Subtree == nil {
			return false, errors.Errorf("subtree for node %v in tree %v is nil", node.Name, p)
		}

		if ignoreTrees.Has(*node.Subtree) {
			continue
		}

		subtree, err := repo.LoadTree(ctx, *node.Subtree)
		ignore, err := walkFn(parentTreeID, p, node, err)
		if err != nil {
			if err == SkipNode {
				if ignore {
					ignoreTrees.Insert(*node.Subtree)
				}
				continue
			}
			return false, err
		}

		if ignore {
			ignoreTrees.Insert(*node.Subtree)
		}

		if !ignore {
			allNodesIgnored = false
		}

		ignore, err = walk(ctx, repo, p, *node.Subtree, subtree, ignoreTrees, walkFn)
		if err != nil {
			return false, err
		}

		if ignore {
			ignoreTrees.Insert(*node.Subtree)
		}

		if !ignore {
			allNodesIgnored = false
		}
	}

	return allNodesIgnored, nil
}
