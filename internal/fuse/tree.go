package fuse

import (
	"context"
	"errors"
	"path"
	"slices"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

// cleanupNodeName returns the last element of name. Node names use "/" as
// separator on all platforms, a backslash is an ordinary character.
func cleanupNodeName(name string) string {
	return path.Base(name)
}

// returning a wrapped context.Canceled error will instead result in returning
// an input / output error to the user. Thus unwrap the error to match the
// expectations of bazil/fuse
func unwrapCtxCanceled(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return err
}

// replaceSpecialNodes replaces nodes with name "." and "/" by their contents.
// Otherwise, the node is returned.
func replaceSpecialNodes(ctx context.Context, repo restic.BlobLoader, node *data.Node) (data.TreeNodeIterator, error) {
	if node.Type != data.NodeTypeDir || node.Subtree == nil {
		return slices.Values([]data.NodeOrError{{Node: node}}), nil
	}

	if node.Name != "." && node.Name != "/" {
		return slices.Values([]data.NodeOrError{{Node: node}}), nil
	}

	tree, err := data.LoadTree(ctx, repo, *node.Subtree)
	if err != nil {
		return nil, unwrapCtxCanceled(err)
	}

	return tree, nil
}

// loadTreeItems loads the tree with the given id and returns its nodes indexed
// by their cleaned up name.
func loadTreeItems(ctx context.Context, repo restic.BlobLoader, id restic.ID) (map[string]*data.Node, error) {
	tree, err := data.LoadTree(ctx, repo, id)
	if err != nil {
		debug.Log("  error loading tree %v: %v", id, err)
		return nil, unwrapCtxCanceled(err)
	}
	items := make(map[string]*data.Node)
	for item := range tree {
		if item.Error != nil {
			return nil, unwrapCtxCanceled(item.Error)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		n := item.Node

		nodes, err := replaceSpecialNodes(ctx, repo, n)
		if err != nil {
			debug.Log("  replaceSpecialNodes(%v) failed: %v", n, err)
			return nil, err
		}
		for item := range nodes {
			if item.Error != nil {
				return nil, unwrapCtxCanceled(item.Error)
			}
			items[cleanupNodeName(item.Node.Name)] = item.Node
		}
	}
	return items, nil
}
