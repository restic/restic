package walker

import (
	"context"
	"fmt"
	"path"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

type NodeRewriteFunc func(node *restic.Node, path string) *restic.Node

type RewriteOpts struct {
	// return nil to remove the node
	RewriteNode NodeRewriteFunc
}

type TreeRewriter struct {
	opts RewriteOpts
}

func NewTreeRewriter(opts RewriteOpts) *TreeRewriter {
	rw := &TreeRewriter{
		opts: opts,
	}
	// setup default implementations
	if rw.opts.RewriteNode == nil {
		rw.opts.RewriteNode = func(node *restic.Node, path string) *restic.Node {
			return node
		}
	}
	return rw
}

type BlobLoadSaver interface {
	restic.BlobSaver
	restic.BlobLoader
}

func (t *TreeRewriter) RewriteTree(ctx context.Context, repo BlobLoadSaver, nodepath string, nodeID restic.ID) (newNodeID restic.ID, err error) {
	curTree, err := restic.LoadTree(ctx, repo, nodeID)
	if err != nil {
		return restic.ID{}, err
	}

	// check that we can properly encode this tree without losing information
	// The alternative of using json/Decoder.DisallowUnknownFields() doesn't work as we use
	// a custom UnmarshalJSON to decode trees, see also https://github.com/golang/go/issues/41144
	testID, err := restic.SaveTree(ctx, repo, curTree)
	if err != nil {
		return restic.ID{}, err
	}
	if nodeID != testID {
		return restic.ID{}, fmt.Errorf("cannot encode tree at %q without losing information", nodepath)
	}

	debug.Log("filterTree: %s, nodeId: %s\n", nodepath, nodeID.Str())

	tb := restic.NewTreeJSONBuilder()
	for _, node := range curTree.Nodes {
		path := path.Join(nodepath, node.Name)
		node = t.opts.RewriteNode(node, path)
		if node == nil {
			continue
		}

		if node.Type != "dir" {
			err = tb.AddNode(node)
			if err != nil {
				return restic.ID{}, err
			}
			continue
		}
		newID, err := t.RewriteTree(ctx, repo, path, *node.Subtree)
		if err != nil {
			return restic.ID{}, err
		}
		node.Subtree = &newID
		err = tb.AddNode(node)
		if err != nil {
			return restic.ID{}, err
		}
	}

	tree, err := tb.Finalize()
	if err != nil {
		return restic.ID{}, err
	}

	// Save new tree
	newTreeID, _, _, err := repo.SaveBlob(ctx, restic.TreeBlob, tree, restic.ID{}, false)
	if !newTreeID.Equal(nodeID) {
		debug.Log("filterTree: save new tree for %s as %v\n", nodepath, newTreeID)
	}
	return newTreeID, err
}
