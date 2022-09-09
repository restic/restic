package walker

import (
	"context"
	"path"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

// SelectByNameFunc returns true for all items that should be included (files and
// dirs). If false is returned, files are ignored and dirs are not even walked.
type SelectByNameFunc func(item string) bool

type TreeFilterVisitor struct {
	SelectByName SelectByNameFunc
	PrintExclude func(string)
}

func FilterTree(ctx context.Context, repo restic.Repository, nodepath string, nodeID restic.ID, visitor *TreeFilterVisitor) (newNodeID restic.ID, err error) {
	curTree, err := restic.LoadTree(ctx, repo, nodeID)
	if err != nil {
		return restic.ID{}, err
	}

	debug.Log("filterTree: %s, nodeId: %s\n", nodepath, nodeID.Str())

	changed := false
	tb := restic.NewTreeJSONBuilder()
	for _, node := range curTree.Nodes {
		path := path.Join(nodepath, node.Name)
		if !visitor.SelectByName(path) {
			if visitor.PrintExclude != nil {
				visitor.PrintExclude(path)
			}
			changed = true
			continue
		}

		if node.Subtree == nil {
			err = tb.AddNode(node)
			if err != nil {
				return restic.ID{}, err
			}
			continue
		}
		newID, err := FilterTree(ctx, repo, path, *node.Subtree, visitor)
		if err != nil {
			return restic.ID{}, err
		}
		if !node.Subtree.Equal(newID) {
			changed = true
		}
		node.Subtree = &newID
		err = tb.AddNode(node)
		if err != nil {
			return restic.ID{}, err
		}
	}

	if changed {
		tree, err := tb.Finalize()
		if err != nil {
			return restic.ID{}, err
		}

		// Save new tree
		newTreeID, _, _, err := repo.SaveBlob(ctx, restic.TreeBlob, tree, restic.ID{}, false)
		debug.Log("filterTree: save new tree for %s as %v\n", nodepath, newTreeID)
		return newTreeID, err
	}

	return nodeID, nil
}
