package restorer

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"

	"golang.org/x/sync/errgroup"
)

// Restorer is used to restore a snapshot to a directory.
type Restorer struct {
	repo   restic.Repository
	sn     *restic.Snapshot
	sparse bool

	Error        func(location string, err error) error
	SelectFilter func(item string, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool)
}

var restorerAbortOnAllErrors = func(location string, err error) error { return err }

// NewRestorer creates a restorer preloaded with the content from the snapshot id.
func NewRestorer(ctx context.Context, repo restic.Repository, sn *restic.Snapshot, sparse bool) *Restorer {
	r := &Restorer{
		repo:         repo,
		sparse:       sparse,
		Error:        restorerAbortOnAllErrors,
		SelectFilter: func(string, string, *restic.Node) (bool, bool) { return true, true },
		sn:           sn,
	}

	return r
}

type treeVisitor struct {
	enterDir  func(node *restic.Node, target, location string) error
	visitNode func(node *restic.Node, target, location string) error
	leaveDir  func(node *restic.Node, target, location string) error
}

// traverseTree traverses a tree from the repo and calls treeVisitor.
// target is the path in the file system, location within the snapshot.
func (res *Restorer) traverseTree(ctx context.Context, target, location string, treeID restic.ID, visitor treeVisitor) (hasRestored bool, err error) {
	debug.Log("%v %v %v", target, location, treeID)
	tree, err := restic.LoadTree(ctx, res.repo, treeID)
	if err != nil {
		debug.Log("error loading tree %v: %v", treeID, err)
		return hasRestored, res.Error(location, err)
	}

	for _, node := range tree.Nodes {

		// ensure that the node name does not contain anything that refers to a
		// top-level directory.
		nodeName := filepath.Base(filepath.Join(string(filepath.Separator), node.Name))
		if nodeName != node.Name {
			debug.Log("node %q has invalid name %q", node.Name, nodeName)
			err := res.Error(location, errors.Errorf("invalid child node name %s", node.Name))
			if err != nil {
				return hasRestored, err
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
				return hasRestored, err
			}
			continue
		}

		// sockets cannot be restored
		if node.Type == "socket" {
			continue
		}

		selectedForRestore, childMayBeSelected := res.SelectFilter(nodeLocation, nodeTarget, node)
		debug.Log("SelectFilter returned %v %v for %q", selectedForRestore, childMayBeSelected, nodeLocation)

		if selectedForRestore {
			hasRestored = true
		}

		sanitizeError := func(err error) error {
			switch err {
			case nil, context.Canceled, context.DeadlineExceeded:
				// Context errors are permanent.
				return err
			default:
				return res.Error(nodeLocation, err)
			}
		}

		if node.Type == "dir" {
			if node.Subtree == nil {
				return hasRestored, errors.Errorf("Dir without subtree in tree %v", treeID.Str())
			}

			if selectedForRestore && visitor.enterDir != nil {
				err = sanitizeError(visitor.enterDir(node, nodeTarget, nodeLocation))
				if err != nil {
					return hasRestored, err
				}
			}

			// keep track of restored child status
			// so metadata of the current directory are restored on leaveDir
			childHasRestored := false

			if childMayBeSelected {
				childHasRestored, err = res.traverseTree(ctx, nodeTarget, nodeLocation, *node.Subtree, visitor)
				err = sanitizeError(err)
				if err != nil {
					return hasRestored, err
				}
				// inform the parent directory to restore parent metadata on leaveDir if needed
				if childHasRestored {
					hasRestored = true
				}
			}

			// metadata need to be restore when leaving the directory in both cases
			// selected for restore or any child of any subtree have been restored
			if (selectedForRestore || childHasRestored) && visitor.leaveDir != nil {
				err = sanitizeError(visitor.leaveDir(node, nodeTarget, nodeLocation))
				if err != nil {
					return hasRestored, err
				}
			}

			continue
		}

		if selectedForRestore {
			err = sanitizeError(visitor.visitNode(node, nodeTarget, nodeLocation))
			if err != nil {
				return hasRestored, err
			}
		}
	}

	return hasRestored, nil
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
		return errors.WithStack(err)
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

	idx := NewHardlinkIndex()
	filerestorer := newFileRestorer(dst, res.repo.Backend().Load, res.repo.Key(), res.repo.Index().Lookup, res.repo.Connections(), res.sparse)
	filerestorer.Error = res.Error

	debug.Log("first pass for %q", dst)

	// first tree pass: create directories and collect all files to restore
	_, err = res.traverseTree(ctx, dst, string(filepath.Separator), *res.sn.Tree, treeVisitor{
		enterDir: func(node *restic.Node, target, location string) error {
			debug.Log("first pass, enterDir: mkdir %q, leaveDir should restore metadata", location)
			// create dir with default permissions
			// #leaveDir restores dir metadata after visiting all children
			return fs.MkdirAll(target, 0700)
		},

		visitNode: func(node *restic.Node, target, location string) error {
			debug.Log("first pass, visitNode: mkdir %q, leaveDir on second pass should restore metadata", location)
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

			filerestorer.addFile(location, node.Content, int64(node.Size))

			return nil
		},
	})
	if err != nil {
		return err
	}

	err = filerestorer.restoreFiles(ctx)
	if err != nil {
		return err
	}

	debug.Log("second pass for %q", dst)

	// second tree pass: restore special files and filesystem metadata
	_, err = res.traverseTree(ctx, dst, string(filepath.Separator), *res.sn.Tree, treeVisitor{
		visitNode: func(node *restic.Node, target, location string) error {
			debug.Log("second pass, visitNode: restore node %q", location)
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
		leaveDir: res.restoreNodeMetadataTo,
	})
	return err
}

// Snapshot returns the snapshot this restorer is configured to use.
func (res *Restorer) Snapshot() *restic.Snapshot {
	return res.sn
}

// Number of workers in VerifyFiles.
const nVerifyWorkers = 8

// VerifyFiles checks whether all regular files in the snapshot res.sn
// have been successfully written to dst. It stops when it encounters an
// error. It returns that error and the number of files it has successfully
// verified.
func (res *Restorer) VerifyFiles(ctx context.Context, dst string) (int, error) {
	type mustCheck struct {
		node *restic.Node
		path string
	}

	var (
		nchecked uint64
		work     = make(chan mustCheck, 2*nVerifyWorkers)
	)

	g, ctx := errgroup.WithContext(ctx)

	// Traverse tree and send jobs to work.
	g.Go(func() error {
		defer close(work)

		_, err := res.traverseTree(ctx, dst, string(filepath.Separator), *res.sn.Tree, treeVisitor{
			visitNode: func(node *restic.Node, target, location string) error {
				if node.Type != "file" {
					return nil
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case work <- mustCheck{node, target}:
					return nil
				}
			},
		})
		return err
	})

	for i := 0; i < nVerifyWorkers; i++ {
		g.Go(func() (err error) {
			var buf []byte
			for job := range work {
				buf, err = res.verifyFile(job.path, job.node, buf)
				if err != nil {
					err = res.Error(job.path, err)
				}
				if err != nil || ctx.Err() != nil {
					break
				}
				atomic.AddUint64(&nchecked, 1)
			}
			return err
		})
	}

	return int(nchecked), g.Wait()
}

// Verify that the file target has the contents of node.
//
// buf and the first return value are scratch space, passed around for reuse.
// Reusing buffers prevents the verifier goroutines allocating all of RAM and
// flushing the filesystem cache (at least on Linux).
func (res *Restorer) verifyFile(target string, node *restic.Node, buf []byte) ([]byte, error) {
	f, err := os.Open(target)
	if err != nil {
		return buf, err
	}
	defer func() {
		_ = f.Close()
	}()

	fi, err := f.Stat()
	switch {
	case err != nil:
		return buf, err
	case int64(node.Size) != fi.Size():
		return buf, errors.Errorf("Invalid file size for %s: expected %d, got %d",
			target, node.Size, fi.Size())
	}

	var offset int64
	for _, blobID := range node.Content {
		length, found := res.repo.LookupBlobSize(blobID, restic.DataBlob)
		if !found {
			return buf, errors.Errorf("Unable to fetch blob %s", blobID)
		}

		if length > uint(cap(buf)) {
			buf = make([]byte, 2*length)
		}
		buf = buf[:length]

		_, err = f.ReadAt(buf, offset)
		if err != nil {
			return buf, err
		}
		if !blobID.Equal(restic.Hash(buf)) {
			return buf, errors.Errorf(
				"Unexpected content in %s, starting at offset %d",
				target, offset)
		}
		offset += int64(length)
	}

	return buf, nil
}
