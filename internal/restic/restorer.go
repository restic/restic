package restic

import (
	"context"
	"os"
	"path/filepath"

	"github.com/restic/restic/internal/errors"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
)

// Restorer is used to restore a snapshot to a directory.
type Restorer struct {
	repo Repository
	sn   *Snapshot

	Error        func(dir string, node *Node, err error) error
	SelectFilter func(item string, dstpath string, node *Node) (selectedForRestore bool, childMayBeSelected bool)
}

var restorerAbortOnAllErrors = func(str string, node *Node, err error) error { return err }

// NewRestorer creates a restorer preloaded with the content from the snapshot id.
func NewRestorer(repo Repository, id ID) (*Restorer, error) {
	r := &Restorer{
		repo: repo, Error: restorerAbortOnAllErrors,
		SelectFilter: func(string, string, *Node) (bool, bool) { return true, true },
	}

	var err error

	r.sn, err = LoadSnapshot(context.TODO(), repo, id)
	if err != nil {
		return nil, err
	}

	return r, nil
}

// restoreTo restores a tree from the repo to a destination. target is the path in
// the file system, location within the snapshot.
func (res *Restorer) restoreTo(ctx context.Context, target, location string, treeID ID, idx *HardlinkIndex) error {
	debug.Log("%v %v %v", target, location, treeID)
	tree, err := res.repo.LoadTree(ctx, treeID)
	if err != nil {
		debug.Log("error loading tree %v: %v", treeID, err)
		return res.Error(location, nil, err)
	}

	for _, node := range tree.Nodes {

		// ensure that the node name does not contain anything that refers to a
		// top-level directory.
		nodeName := filepath.Base(filepath.Join(string(filepath.Separator), node.Name))
		if nodeName != node.Name {
			debug.Log("node %q has invalid name %q", node.Name, nodeName)
			err := res.Error(location, node, errors.New("node has invalid name"))
			if err != nil {
				return err
			}
			continue
		}

		nodeTarget := filepath.Join(target, nodeName)
		nodeLocation := filepath.Join(location, nodeName)

		if target == nodeTarget || !fs.HasPathPrefix(target, nodeTarget) {
			debug.Log("target: %v %v", target, nodeTarget)
			debug.Log("node %q has invalid target path %q", node.Name, nodeTarget)
			err := res.Error(nodeLocation, node, errors.New("node has invalid path"))
			if err != nil {
				return err
			}
			continue
		}

		// sockets cannot be restored
		if node.Type == "socket" {
			continue
		}

		selectedForRestore, childMayBeSelected := res.SelectFilter(nodeLocation, nodeTarget, node)
		debug.Log("SelectFilter returned %v %v", selectedForRestore, childMayBeSelected)

		if node.Type == "dir" && childMayBeSelected {
			if node.Subtree == nil {
				return errors.Errorf("Dir without subtree in tree %v", treeID.Str())
			}

			err = res.restoreTo(ctx, nodeTarget, nodeLocation, *node.Subtree, idx)
			if err != nil {
				err = res.Error(nodeLocation, node, err)
				if err != nil {
					return err
				}
			}
		}

		if selectedForRestore {
			err = res.restoreNodeTo(ctx, node, nodeTarget, nodeLocation, idx)
			if err != nil {
				err = res.Error(nodeLocation, node, errors.Wrap(err, "restoreNodeTo"))
				if err != nil {
					return err
				}
			}

			// Restore directory timestamp at the end. If we would do it earlier, restoring files within
			// the directory would overwrite the timestamp of the directory they are in.
			err = node.RestoreTimestamps(nodeTarget)
			if err != nil {
				err = res.Error(nodeLocation, node, errors.Wrap(err, "RestoreTimestamps"))
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (res *Restorer) restoreNodeTo(ctx context.Context, node *Node, target, location string, idx *HardlinkIndex) error {
	debug.Log("%v %v %v", node.Name, target, location)

	err := node.CreateAt(ctx, target, res.repo, idx)
	if err != nil {
		debug.Log("node.CreateAt(%s) error %v", target, err)
	}

	// Did it fail because of ENOENT?
	if err != nil && os.IsNotExist(errors.Cause(err)) {
		debug.Log("create intermediate paths")

		// Create parent directories and retry
		err = fs.MkdirAll(filepath.Dir(target), 0700)
		if err == nil || os.IsExist(errors.Cause(err)) {
			err = node.CreateAt(ctx, target, res.repo, idx)
		}
	}

	if err != nil {
		debug.Log("error %v", err)
		err = res.Error(location, node, err)
		if err != nil {
			return err
		}
	}

	return nil
}

// RestoreTo creates the directories and files in the snapshot below dst.
// Before an item is created, res.Filter is called.
func (res *Restorer) RestoreTo(ctx context.Context, dst string) error {
	var err error
	if !filepath.IsAbs(dst) {
		dst, err = filepath.Abs(dst)
		if err != nil {
			return errors.Wrap(err, "Abs")
		}
	}

	idx := NewHardlinkIndex()
	return res.restoreTo(ctx, dst, string(filepath.Separator), *res.sn.Tree, idx)
}

// Snapshot returns the snapshot this restorer is configured to use.
func (res *Restorer) Snapshot() *Snapshot {
	return res.sn
}
