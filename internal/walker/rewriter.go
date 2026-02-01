package walker

import (
	"context"
	"fmt"
	"path"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

type NodeRewriteFunc func(node *data.Node, path string) *data.Node
type FailedTreeRewriteFunc func(nodeID restic.ID, path string, err error) (data.TreeNodeIterator, error)
type QueryRewrittenSizeFunc func() SnapshotSize
type NodeKeepEmptyDirectoryFunc func(path string) bool

type SnapshotSize struct {
	FileCount uint
	FileSize  uint64
}

type RewriteOpts struct {
	// return nil to remove the node
	RewriteNode        NodeRewriteFunc
	KeepEmptyDirectory NodeKeepEmptyDirectoryFunc
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
		rw.opts.RewriteNode = func(node *data.Node, _ string) *data.Node {
			return node
		}
	}
	if rw.opts.RewriteFailedTree == nil {
		// fail with error by default
		rw.opts.RewriteFailedTree = func(_ restic.ID, _ string, err error) (data.TreeNodeIterator, error) {
			return nil, err
		}
	}
	if rw.opts.KeepEmptyDirectory == nil {
		rw.opts.KeepEmptyDirectory = func(_ string) bool {
			return true
		}
	}
	return rw
}

func NewSnapshotSizeRewriter(rewriteNode NodeRewriteFunc, keepEmptyDirecoryFilter NodeKeepEmptyDirectoryFunc) (*TreeRewriter, QueryRewrittenSizeFunc) {
	var count uint
	var size uint64

	t := NewTreeRewriter(RewriteOpts{
		RewriteNode: func(node *data.Node, path string) *data.Node {
			node = rewriteNode(node, path)
			if node != nil && node.Type == data.NodeTypeFile {
				count++
				size += node.Size
			}
			return node
		},
		DisableNodeCache:   true,
		KeepEmptyDirectory: keepEmptyDirecoryFilter,
	})

	ss := func() SnapshotSize {
		return SnapshotSize{count, size}
	}

	return t, ss
}

func (t *TreeRewriter) RewriteTree(ctx context.Context, loader restic.BlobLoader, saver restic.BlobSaver, nodepath string, nodeID restic.ID) (newNodeID restic.ID, err error) {
	// check if tree was already changed
	newID, ok := t.replaces[nodeID]
	if ok {
		return newID, nil
	}

	// a nil nodeID will lead to a load error
	curTree, err := data.LoadTree(ctx, loader, nodeID)
	if err != nil {
		replacement, err := t.opts.RewriteFailedTree(nodeID, nodepath, err)
		if err != nil {
			return restic.ID{}, err
		}
		if replacement != nil {
			replacementID, err := data.SaveTree(ctx, saver, replacement)
			if err != nil {
				return restic.ID{}, err
			}
			return replacementID, nil
		}
		return restic.ID{}, nil
	}

	if !t.opts.AllowUnstableSerialization {
		// check that we can properly encode this tree without losing information
		// The alternative of using json/Decoder.DisallowUnknownFields() doesn't work as we use
		// a custom UnmarshalJSON to decode trees, see also https://github.com/golang/go/issues/41144
		testID, err := data.SaveTree(ctx, saver, curTree)
		if err != nil {
			return restic.ID{}, err
		}
		if nodeID != testID {
			return restic.ID{}, fmt.Errorf("cannot encode tree at %q without losing information", nodepath)
		}

		// reload the tree to get a new iterator
		curTree, err = data.LoadTree(ctx, loader, nodeID)
		if err != nil {
			// shouldn't fail as the first load was successful
			return restic.ID{}, fmt.Errorf("failed to reload tree %v: %w", nodeID, err)
		}
	}

	debug.Log("filterTree: %s, nodeId: %s\n", nodepath, nodeID.Str())

	tb := data.NewTreeWriter(saver)
	for item := range curTree {
		if ctx.Err() != nil {
			return restic.ID{}, ctx.Err()
		}
		if item.Error != nil {
			return restic.ID{}, item.Error
		}
		node := item.Node

		path := path.Join(nodepath, node.Name)
		node = t.opts.RewriteNode(node, path)
		if node == nil {
			continue
		}

		if node.Type != data.NodeTypeDir {
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
		newID, err := t.RewriteTree(ctx, loader, saver, path, subtree)
		if err != nil {
			return restic.ID{}, err
		} else if err == nil && newID.IsNull() {
			continue
		}
		node.Subtree = &newID
		err = tb.AddNode(node)
		if err != nil {
			return restic.ID{}, err
		}
	}

	newTreeID, err := tb.Finalize(ctx)
	if err != nil {
		return restic.ID{}, err
	}
	if tb.Count() == 0 && !t.opts.KeepEmptyDirectory(nodepath) {
		return restic.ID{}, nil
	}

	if t.replaces != nil {
		t.replaces[nodeID] = newTreeID
	}
	if !newTreeID.Equal(nodeID) {
		debug.Log("filterTree: save new tree for %s as %v\n", nodepath, newTreeID)
	}
	return newTreeID, err
}
