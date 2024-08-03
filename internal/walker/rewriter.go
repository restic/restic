package walker

import (
	"context"
	"fmt"
	"path"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

type NodeRewriteFunc func(node *restic.Node, path string) *restic.Node
type FailedTreeRewriteFunc func(nodeID restic.ID, path string, err error) (restic.ID, error)
type QueryRewrittenSizeFunc func() SnapshotSize

type SnapshotSize struct {
	FileCount uint
	FileSize  uint64
}

type RewriteOpts struct {
	// return nil to remove the node
	RewriteNode NodeRewriteFunc
	// decide what to do with a tree that could not be loaded. Return nil to remove the node. By default the load error is returned which causes the operation to fail.
	RewriteFailedTree FailedTreeRewriteFunc

	AllowUnstableSerialization bool
	DisableNodeCache           bool
}

type idMap map[restic.ID]restic.ID

type TreeRewriter struct {
	opts RewriteOpts

	replaces idMap
}

func NewTreeRewriter(opts RewriteOpts) *TreeRewriter {
	rw := &TreeRewriter{
		opts: opts,
	}
	if !opts.DisableNodeCache {
		rw.replaces = make(idMap)
	}
	// setup default implementations
	if rw.opts.RewriteNode == nil {
		rw.opts.RewriteNode = func(node *restic.Node, _ string) *restic.Node {
			return node
		}
	}
	if rw.opts.RewriteFailedTree == nil {
		// fail with error by default
		rw.opts.RewriteFailedTree = func(_ restic.ID, _ string, err error) (restic.ID, error) {
			return restic.ID{}, err
		}
	}
	return rw
}

func NewSnapshotSizeRewriter(rewriteNode NodeRewriteFunc) (*TreeRewriter, QueryRewrittenSizeFunc) {
	var count uint
	var size uint64

	t := NewTreeRewriter(RewriteOpts{
		RewriteNode: func(node *restic.Node, path string) *restic.Node {
			node = rewriteNode(node, path)
			if node != nil && node.Type == "file" {
				count++
				size += node.Size
			}
			return node
		},
		DisableNodeCache: true,
	})

	ss := func() SnapshotSize {
		return SnapshotSize{count, size}
	}

	return t, ss
}

type BlobLoadSaver interface {
	restic.BlobSaver
	restic.BlobLoader
}

func (t *TreeRewriter) RewriteTree(ctx context.Context, repo BlobLoadSaver, nodepath string, nodeID restic.ID) (newNodeID restic.ID, err error) {
	// check if tree was already changed
	newID, ok := t.replaces[nodeID]
	if ok {
		return newID, nil
	}

	// a nil nodeID will lead to a load error
	curTree, err := restic.LoadTree(ctx, repo, nodeID)
	if err != nil {
		return t.opts.RewriteFailedTree(nodeID, nodepath, err)
	}

	if !t.opts.AllowUnstableSerialization {
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
	}

	debug.Log("filterTree: %s, nodeId: %s\n", nodepath, nodeID.Str())

	tb := restic.NewTreeJSONBuilder()
	for _, node := range curTree.Nodes {
		if ctx.Err() != nil {
			return restic.ID{}, ctx.Err()
		}

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
		// treat nil as null id
		var subtree restic.ID
		if node.Subtree != nil {
			subtree = *node.Subtree
		}
		newID, err := t.RewriteTree(ctx, repo, path, subtree)
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
	if t.replaces != nil {
		t.replaces[nodeID] = newTreeID
	}
	if !newTreeID.Equal(nodeID) {
		debug.Log("filterTree: save new tree for %s as %v\n", nodepath, newTreeID)
	}
	return newTreeID, err
}
