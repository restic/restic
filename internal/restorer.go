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
	SelectFilter func(item string, dstpath string, node *Node) bool
}

var restorerAbortOnAllErrors = func(str string, node *Node, err error) error { return err }

// NewRestorer creates a restorer preloaded with the content from the snapshot id.
func NewRestorer(repo Repository, id ID) (*Restorer, error) {
	r := &Restorer{
		repo: repo, Error: restorerAbortOnAllErrors,
		SelectFilter: func(string, string, *Node) bool { return true },
	}

	var err error

	r.sn, err = LoadSnapshot(context.TODO(), repo, id)
	if err != nil {
		return nil, err
	}

	return r, nil
}

func (res *Restorer) restoreTo(ctx context.Context, dst string, dir string, treeID ID, idx *HardlinkIndex) error {
	tree, err := res.repo.LoadTree(ctx, treeID)
	if err != nil {
		return res.Error(dir, nil, err)
	}

	for _, node := range tree.Nodes {
		selectedForRestore := res.SelectFilter(filepath.Join(dir, node.Name),
			filepath.Join(dst, dir, node.Name), node)
		debug.Log("SelectForRestore returned %v", selectedForRestore)

		if selectedForRestore {
			err := res.restoreNodeTo(ctx, node, dir, dst, idx)
			if err != nil {
				return err
			}
		}

		if node.Type == "dir" {
			if node.Subtree == nil {
				return errors.Errorf("Dir without subtree in tree %v", treeID.Str())
			}

			subp := filepath.Join(dir, node.Name)
			err = res.restoreTo(ctx, dst, subp, *node.Subtree, idx)
			if err != nil {
				err = res.Error(subp, node, err)
				if err != nil {
					return err
				}
			}

			if selectedForRestore {
				// Restore directory timestamp at the end. If we would do it earlier, restoring files within
				// the directory would overwrite the timestamp of the directory they are in.
				if err := node.RestoreTimestamps(filepath.Join(dst, dir, node.Name)); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (res *Restorer) restoreNodeTo(ctx context.Context, node *Node, dir string, dst string, idx *HardlinkIndex) error {
	debug.Log("node %v, dir %v, dst %v", node.Name, dir, dst)
	dstPath := filepath.Join(dst, dir, node.Name)

	err := node.CreateAt(ctx, dstPath, res.repo, idx)
	if err != nil {
		debug.Log("node.CreateAt(%s) error %v", dstPath, err)
	}

	// Did it fail because of ENOENT?
	if err != nil && os.IsNotExist(errors.Cause(err)) {
		debug.Log("create intermediate paths")

		// Create parent directories and retry
		err = fs.MkdirAll(filepath.Dir(dstPath), 0700)
		if err == nil || os.IsExist(errors.Cause(err)) {
			err = node.CreateAt(ctx, dstPath, res.repo, idx)
		}
	}

	if err != nil {
		debug.Log("error %v", err)
		err = res.Error(dstPath, node, err)
		if err != nil {
			return err
		}
	}

	debug.Log("successfully restored %v", node.Name)

	return nil
}

// RestoreTo creates the directories and files in the snapshot below dst.
// Before an item is created, res.Filter is called.
func (res *Restorer) RestoreTo(ctx context.Context, dst string) error {
	idx := NewHardlinkIndex()
	return res.restoreTo(ctx, dst, string(filepath.Separator), *res.sn.Tree, idx)
}

// Snapshot returns the snapshot this restorer is configured to use.
func (res *Restorer) Snapshot() *Snapshot {
	return res.sn
}
