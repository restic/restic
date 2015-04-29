package restic

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/juju/arrar"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/server"
)

type Restorer struct {
	s  *server.Server
	sn *Snapshot

	Error  func(dir string, node *Node, err error) error
	Filter func(item string, dstpath string, node *Node) bool
}

// NewRestorer creates a restorer preloaded with the content from the snapshot snid.
func NewRestorer(s *server.Server, snid backend.ID) (*Restorer, error) {
	r := &Restorer{s: s}

	var err error

	r.sn, err = LoadSnapshot(s, snid)
	if err != nil {
		return nil, arrar.Annotate(err, "load snapshot for restorer")
	}

	// abort on all errors
	r.Error = func(string, *Node, error) error { return err }

	return r, nil
}

func (res *Restorer) to(dst string, dir string, treeID backend.ID) error {
	tree, err := LoadTree(res.s, treeID)
	if err != nil {
		return res.Error(dir, nil, arrar.Annotate(err, "LoadTree"))
	}

	for _, node := range tree.Nodes {
		dstpath := filepath.Join(dst, dir, node.Name)

		if res.Filter == nil ||
			res.Filter(filepath.Join(dir, node.Name), dstpath, node) {
			err := node.CreateAt(dstpath, res.s)

			// Did it fail because of ENOENT?
			if arrar.Check(err, func(err error) bool {
				if pe, ok := err.(*os.PathError); ok {
					errn, ok := pe.Err.(syscall.Errno)
					return ok && errn == syscall.ENOENT
				}
				return false
			}) {
				// Create parent directories and retry
				err = os.MkdirAll(filepath.Dir(dstpath), 0700)
				if err == nil || err == os.ErrExist {
					err = node.CreateAt(dstpath, res.s)
				}
			}

			if err != nil {
				err = res.Error(dstpath, node, arrar.Annotate(err, "create node"))
				if err != nil {
					return err
				}
			}
		}

		if node.Type == "dir" {
			if node.Subtree == nil {
				return fmt.Errorf("Dir without subtree in tree %v", treeID.Str())
			}

			subp := filepath.Join(dir, node.Name)
			err = res.to(dst, subp, node.Subtree)
			if err != nil {
				err = res.Error(subp, node, arrar.Annotate(err, "restore subtree"))
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// RestoreTo creates the directories and files in the snapshot below dir.
// Before an item is created, res.Filter is called.
func (res *Restorer) RestoreTo(dir string) error {
	return res.to(dir, "", res.sn.Tree)
}

func (res *Restorer) Snapshot() *Snapshot {
	return res.sn
}
