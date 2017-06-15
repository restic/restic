package walk

import (
	"context"
	"path/filepath"
	"restic"
	"restic/debug"
	"restic/errors"
	"sort"
)

// Func is the type of the function called for each tree. The dir argument
// contains a slash-separated path from the root of the tree. If there was a
// problem loading the tree, tree may be nil and error is set accordingly. If a
// non-nil error is returned, walking stops.
type Func func(dir string, id restic.ID, tree *restic.Tree, err error) error

// Walk walks the tree with treeID by loading subsequent subtrees from the repo,
// calling fn for each tree. The walk order is determined by the subdir names,
// depths first. Walk returns the first non-nil error returned by fn. If ctx is
// cancelled, walking stops.
func Walk(ctx context.Context, repo TreeLoader, treeID restic.ID, fn Func) error {
	return walk(ctx, repo, treeID, "", fn)
}

func walk(ctx context.Context, repo TreeLoader, treeID restic.ID, dir string, fn Func) error {
	debug.Log("walk %v", treeID.Str())

	// if context has been cancelled, return with an error.
	if err := ctx.Err(); err != nil {
		return err
	}

	tree, err := repo.LoadTree(ctx, treeID)
	err = fn(dir, treeID, tree, err)
	if err != nil {
		return err
	}

	sort.Sort(restic.Nodes(tree.Nodes))
	for _, node := range tree.Nodes {
		if node.Type != "dir" {
			continue
		}

		if node.Subtree == nil {
			return errors.Errorf("tree %v: node %q has no subtree", treeID, node.Name)
		}

		subdir := filepath.Join(dir, node.Name)
		err := walk(ctx, repo, *node.Subtree, subdir, fn)
		if err != nil {
			return err
		}
	}

	return nil
}
