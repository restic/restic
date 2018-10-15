package restorer

import (
	"context"
	"os"
	"path/filepath"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/errors"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

// Restorer is used to restore a snapshot to a directory.
type Restorer struct {
	repo restic.Repository
	sn   *restic.Snapshot

	Error        func(location string, err error) error
	SelectFilter func(item string, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool)
}

var restorerAbortOnAllErrors = func(location string, err error) error { return err }

// NewRestorer creates a restorer preloaded with the content from the snapshot id.
func NewRestorer(repo restic.Repository, id restic.ID) (*Restorer, error) {
	r := &Restorer{
		repo:         repo,
		Error:        restorerAbortOnAllErrors,
		SelectFilter: func(string, string, *restic.Node) (bool, bool) { return true, true },
	}

	var err error

	r.sn, err = restic.LoadSnapshot(context.TODO(), repo, id)
	if err != nil {
		return nil, err
	}

	return r, nil
}

type treeVisitor struct {
	enterDir  func(node *restic.Node, target, location string) error
	visitNode func(node *restic.Node, target, location string) error
	leaveDir  func(node *restic.Node, target, location string) error
}

// traverseTree traverses a tree from the repo and calls treeVisitor.
// target is the path in the file system, location within the snapshot.
func (res *Restorer) traverseTree(ctx context.Context, target, location string, treeID restic.ID, visitor treeVisitor) error {
	debug.Log("%v %v %v", target, location, treeID)
	tree, err := res.repo.LoadTree(ctx, treeID)
	if err != nil {
		debug.Log("error loading tree %v: %v", treeID, err)
		return res.Error(location, err)
	}

	for _, node := range tree.Nodes {

		// ensure that the node name does not contain anything that refers to a
		// top-level directory.
		nodeName := filepath.Base(filepath.Join(string(filepath.Separator), node.Name))
		if nodeName != node.Name {
			debug.Log("node %q has invalid name %q", node.Name, nodeName)
			err := res.Error(location, errors.Errorf("invalid child node name %s", node.Name))
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
			err := res.Error(nodeLocation, errors.New("node has invalid path"))
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

		sanitizeError := func(err error) error {
			if err != nil {
				err = res.Error(nodeLocation, err)
			}
			return err
		}

		if node.Type == "dir" {
			if node.Subtree == nil {
				return errors.Errorf("Dir without subtree in tree %v", treeID.Str())
			}

			if selectedForRestore {
				err = sanitizeError(visitor.enterDir(node, nodeTarget, nodeLocation))
				if err != nil {
					return err
				}
			}

			if childMayBeSelected {
				err = sanitizeError(res.traverseTree(ctx, nodeTarget, nodeLocation, *node.Subtree, visitor))
				if err != nil {
					return err
				}
			}

			if selectedForRestore {
				err = sanitizeError(visitor.leaveDir(node, nodeTarget, nodeLocation))
				if err != nil {
					return err
				}
			}

			continue
		}

		if selectedForRestore {
			err = sanitizeError(visitor.visitNode(node, nodeTarget, nodeLocation))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (res *Restorer) restoreNodeTo(ctx context.Context, node *restic.Node, target, location string) error {
	debug.Log("restoreNode %v %v %v", node.Name, target, location)

	err := node.CreateAt(ctx, target, res.repo)
	if err != nil {
		debug.Log("node.CreateAt(%s) error %v", target, err)
	}
	if err == nil {
		err = res.restoreNodeMetadataTo(node, target, location)
	}

	return err
}

func (res *Restorer) restoreNodeMetadataTo(node *restic.Node, target, location string) error {
	debug.Log("restoreNodeMetadata %v %v %v", node.Name, target, location)
	err := node.RestoreMetadata(target)
	if err != nil {
		debug.Log("node.RestoreMetadata(%s) error %v", target, err)
	}
	return err
}

func (res *Restorer) restoreHardlinkAt(node *restic.Node, target, path, location string) error {
	if err := fs.Remove(path); !os.IsNotExist(err) {
		return errors.Wrap(err, "RemoveCreateHardlink")
	}
	err := fs.Link(target, path)
	if err != nil {
		return errors.Wrap(err, "CreateHardlink")
	}
	// TODO investigate if hardlinks have separate metadata on any supported system
	return res.restoreNodeMetadataTo(node, path, location)
}

func (res *Restorer) restoreEmptyFileAt(node *restic.Node, target, location string) error {
	wr, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	err = wr.Close()
	if err != nil {
		return err
	}

	return res.restoreNodeMetadataTo(node, target, location)
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

	restoreNodeMetadata := func(node *restic.Node, target, location string) error {
		return res.restoreNodeMetadataTo(node, target, location)
	}
	noop := func(node *restic.Node, target, location string) error { return nil }

	idx := restic.NewHardlinkIndex()

	filerestorer := newFileRestorer(dst, res.repo.Backend().Load, res.repo.Key(), filePackTraverser{lookup: res.repo.Index().Lookup})

	// first tree pass: create directories and collect all files to restore
	err = res.traverseTree(ctx, dst, string(filepath.Separator), *res.sn.Tree, treeVisitor{
		enterDir: func(node *restic.Node, target, location string) error {
			// create dir with default permissions
			// #leaveDir restores dir metadata after visiting all children
			return fs.MkdirAll(target, 0700)
		},

		visitNode: func(node *restic.Node, target, location string) error {
			// create parent dir with default permissions
			// second pass #leaveDir restores dir metadata after visiting/restoring all children
			err := fs.MkdirAll(filepath.Dir(target), 0700)
			if err != nil {
				return err
			}

			if node.Type != "file" {
				return nil
			}

			if node.Size == 0 {
				return nil // deal with empty files later
			}

			if node.Links > 1 {
				if idx.Has(node.Inode, node.DeviceID) {
					return nil
				}
				idx.Add(node.Inode, node.DeviceID, location)
			}

			filerestorer.addFile(location, node.Content)

			return nil
		},
		leaveDir: noop,
	})
	if err != nil {
		return err
	}

	err = filerestorer.restoreFiles(ctx, func(location string, err error) { res.Error(location, err) })
	if err != nil {
		return err
	}

	// second tree pass: restore special files and filesystem metadata
	return res.traverseTree(ctx, dst, string(filepath.Separator), *res.sn.Tree, treeVisitor{
		enterDir: noop,
		visitNode: func(node *restic.Node, target, location string) error {
			if node.Type != "file" {
				return res.restoreNodeTo(ctx, node, target, location)
			}

			// create empty files, but not hardlinks to empty files
			if node.Size == 0 && (node.Links < 2 || !idx.Has(node.Inode, node.DeviceID)) {
				if node.Links > 1 {
					idx.Add(node.Inode, node.DeviceID, location)
				}
				return res.restoreEmptyFileAt(node, target, location)
			}

			if idx.Has(node.Inode, node.DeviceID) && idx.GetFilename(node.Inode, node.DeviceID) != location {
				return res.restoreHardlinkAt(node, filerestorer.targetPath(idx.GetFilename(node.Inode, node.DeviceID)), target, location)
			}

			return res.restoreNodeMetadataTo(node, target, location)
		},
		leaveDir: restoreNodeMetadata,
	})
}

// Snapshot returns the snapshot this restorer is configured to use.
func (res *Restorer) Snapshot() *restic.Snapshot {
	return res.sn
}

// VerifyFiles reads all snapshot files and verifies their contents
func (res *Restorer) VerifyFiles(ctx context.Context, dst string) (int, error) {
	// TODO multithreaded?

	count := 0
	err := res.traverseTree(ctx, dst, string(filepath.Separator), *res.sn.Tree, treeVisitor{
		enterDir: func(node *restic.Node, target, location string) error { return nil },
		visitNode: func(node *restic.Node, target, location string) error {
			if node.Type != "file" {
				return nil
			}

			count++
			stat, err := os.Stat(target)
			if err != nil {
				return err
			}
			if int64(node.Size) != stat.Size() {
				return errors.Errorf("Invalid file size: expected %d got %d", node.Size, stat.Size())
			}

			file, err := os.Open(target)
			if err != nil {
				return err
			}

			offset := int64(0)
			for _, blobID := range node.Content {
				blobs, _ := res.repo.Index().Lookup(blobID, restic.DataBlob)
				length := blobs[0].Length - uint(crypto.Extension)
				buf := make([]byte, length) // TODO do I want to reuse the buffer somehow?
				_, err = file.ReadAt(buf, offset)
				if err != nil {
					_ = file.Close()
					return err
				}
				if !blobID.Equal(restic.Hash(buf)) {
					_ = file.Close()
					return errors.Errorf("Unexpected contents starting at offset %d", offset)
				}
				offset += int64(length)
			}

			err = file.Close()
			if err != nil {
				return err
			}

			return nil
		},
		leaveDir: func(node *restic.Node, target, location string) error { return nil },
	})

	return count, err
}
