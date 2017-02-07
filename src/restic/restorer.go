package restic

import (
	"os"
	"path/filepath"

	"restic/errors"

	"restic/debug"
	"restic/fs"
)

// Restorer is used to restore a snapshot to a directory.
type Restorer struct {
	repo        Repository
	sn          *Snapshot
	progressBar *Progress

	Error        func(dir string, node *Node, err error) error
	SelectFilter func(item string, node *Node) bool
}

var restorerAbortOnAllErrors = func(str string, node *Node, err error) error { return err }

// NewRestorer creates a restorer preloaded with the content from the snapshot id.
func NewRestorer(repo Repository, id ID) (*Restorer, error) {
	r := &Restorer{
		repo:         repo,
		Error:        restorerAbortOnAllErrors,
		SelectFilter: func(string, *Node) bool { return true },
	}

	var err error

	r.sn, err = LoadSnapshot(repo, id)
	if err != nil {
		return nil, err
	}

	return r, nil
}

// Scan traverses the directories/files to be restored to collect restic.Stat information
func (res *Restorer) Scan(p *Progress) (Stat, error) {
	p.Start()
	defer p.Done()

	var stat Stat

	err := res.walk("", *res.sn.Tree, func(node *Node, dir string) error {
		s := Stat{}
		if node.Type == "dir" {
			s.Dirs++
		} else {
			s.Files++
			s.Bytes += node.Size
		}

		p.Report(s)
		stat.Add(s)

		return nil
	})

	return stat, err
}

func (res *Restorer) walk(dir string, treeID ID, callback func(*Node, string) error) error {
	tree, err := res.repo.LoadTree(treeID)
	if err != nil {
		return res.Error(dir, nil, err)
	}

	for _, node := range tree.Nodes {
		selectedForRestore := res.SelectFilter(filepath.Join(dir, node.Name), node)
		debug.Log("SelectForRestore returned %v", selectedForRestore)

		if selectedForRestore {
			err := callback(node, dir)
			if err != nil {
				return err
			}
		}

		if node.Type == "dir" {
			if node.Subtree == nil {
				return errors.Errorf("Dir without subtree in tree %v", treeID.Str())
			}

			subp := filepath.Join(dir, node.Name)
			err = res.walk(subp, *node.Subtree, callback)
			if err != nil {
				err = res.Error(subp, node, err)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (res *Restorer) restoreTo(dst string, dir string, treeID ID) error {
	err := res.walk(dir, treeID, func(node *Node, currentDir string) error {
		return res.restoreNodeTo(node, currentDir, dst)
	})

	if err != nil {
		return err
	}

	// Restore directory timestamps at the end. If we would do it earlier, restoring files within
	// the directory would overwrite the timestamp of the directory they are in.
	err = res.walk(dir, treeID, func(node *Node, currentDir string) error {
		if node.Type == "dir" {
			return node.RestoreTimestamps(filepath.Join(dst, currentDir, node.Name))
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

func (res *Restorer) restoreNodeTo(node *Node, dir string, dst string) error {
	debug.Log("node %v, dir %v, dst %v", node.Name, dir, dst)
	dstPath := filepath.Join(dst, dir, node.Name)

	err := node.CreateAt(dstPath, res.repo, res.progressBar)
	if err != nil {
		debug.Log("node.CreateAt(%s) error %v", dstPath, err)
	}

	// Did it fail because of ENOENT?
	if err != nil && os.IsNotExist(errors.Cause(err)) {
		debug.Log("create intermediate paths")

		// Create parent directories and retry
		err = fs.MkdirAll(filepath.Dir(dstPath), 0700)
		if err == nil || os.IsExist(errors.Cause(err)) {
			err = node.CreateAt(dstPath, res.repo, res.progressBar)
		}
	}

	if err != nil {
		debug.Log("error %v", err)
		res.progressBar.Report(Stat{Errors: 1})
		err = res.Error(dstPath, node, err)
		if err != nil {
			return err
		}
	}

	if node.Type == "dir" {
		res.progressBar.Report(Stat{Dirs: 1})
	} else {
		res.progressBar.Report(Stat{Files: 1})
	}

	debug.Log("successfully restored %v", node.Name)

	return nil
}

// RestoreTo creates the directories and files in the snapshot below dir.
// Before an item is created, res.Filter is called.
func (res *Restorer) RestoreTo(dir string, p *Progress) error {
	res.progressBar = p
	res.progressBar.Start()
	defer res.progressBar.Done()

	return res.restoreTo(dir, "", *res.sn.Tree)
}

// Snapshot returns the snapshot this restorer is configured to use.
func (res *Restorer) Snapshot() *Snapshot {
	return res.sn
}
