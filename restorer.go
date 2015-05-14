package restic

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/repository"

	"github.com/juju/errors"
)

// Restorer is used to restore a snapshot to a directory.
type Restorer struct {
	repo *repository.Repository
	sn   *Snapshot

	Error  func(dir string, node *Node, err error) error
	Filter func(item string, dstpath string, node *Node) bool
}

var restorerAbortOnAllErrors = func(str string, node *Node, err error) error { return err }

// NewRestorer creates a restorer preloaded with the content from the snapshot id.
func NewRestorer(repo *repository.Repository, id backend.ID) (*Restorer, error) {
	r := &Restorer{repo: repo, Error: restorerAbortOnAllErrors}

	var err error

	r.sn, err = LoadSnapshot(repo, id)
	if err != nil {
		return nil, errors.Annotate(err, "load snapshot for restorer")
	}

	return r, nil
}

func (res *Restorer) restoreTo(dst string, dir string, treeID backend.ID) error {
	tree, err := LoadTree(res.repo, treeID)
	if err != nil {
		return res.Error(dir, nil, errors.Annotate(err, "LoadTree"))
	}

	for _, node := range tree.Nodes {
		if err := res.restoreNodeTo(node, dir, dst); err != nil {
			return err
		}

		if node.Type == "dir" {
			if node.Subtree == nil {
				return fmt.Errorf("Dir without subtree in tree %v", treeID.Str())
			}

			subp := filepath.Join(dir, node.Name)
			err = res.restoreTo(dst, subp, node.Subtree)
			if err != nil {
				err = res.Error(subp, node, errors.Annotate(err, "restore subtree"))
				if err != nil {
					return err
				}
			}
		}
	}

	// Restore directory timestamps at the end. If we would do it earlier, restoring files within
	// those directories would overwrite the timestamp of the directories they are in.
	for _, node := range tree.Nodes {
		if node.Type != "dir" {
			continue
		}

		if err := node.RestoreTimestamps(filepath.Join(dst, dir, node.Name)); err != nil {
			return err
		}
	}

	return nil
}

func (res *Restorer) restoreNodeTo(node *Node, dir string, dst string) error {
	dstPath := filepath.Join(dst, dir, node.Name)

	if res.Filter != nil && res.Filter(filepath.Join(dir, node.Name), dstPath, node) {
		return nil
	}

	err := node.CreateAt(dstPath, res.repo)

	// Did it fail because of ENOENT?
	if pe, ok := errors.Cause(err).(*os.PathError); ok {
		errn, ok := pe.Err.(syscall.Errno)
		if ok && errn == syscall.ENOENT {
			// Create parent directories and retry
			err = os.MkdirAll(filepath.Dir(dstPath), 0700)
			if err == nil || err == os.ErrExist {
				err = node.CreateAt(dstPath, res.repo)
			}
		}
	}

	if err != nil {
		err = res.Error(dstPath, node, errors.Annotate(err, "create node"))
		if err != nil {
			return err
		}
	}

	return nil
}

// RestoreTo creates the directories and files in the snapshot below dir.
// Before an item is created, res.Filter is called.
func (res *Restorer) RestoreTo(dir string) error {
	return res.restoreTo(dir, "", res.sn.Tree)
}

// Snapshot returns the snapshot this restorer is configured to use.
func (res *Restorer) Snapshot() *Snapshot {
	return res.sn
}
