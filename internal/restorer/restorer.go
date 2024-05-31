package restorer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	restoreui "github.com/restic/restic/internal/ui/restore"

	"golang.org/x/sync/errgroup"
)

// Restorer is used to restore a snapshot to a directory.
type Restorer struct {
	repo      restic.Repository
	sn        *restic.Snapshot
	sparse    bool
	progress  *restoreui.Progress
	overwrite OverwriteBehavior

	fileList map[string]struct{}

	Error        func(location string, err error) error
	Warn         func(message string)
	SelectFilter func(item string, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool)
}

var restorerAbortOnAllErrors = func(_ string, err error) error { return err }

type Options struct {
	Sparse    bool
	Progress  *restoreui.Progress
	Overwrite OverwriteBehavior
}

type OverwriteBehavior int

// Constants for different overwrite behavior
const (
	OverwriteAlways  OverwriteBehavior = 0
	OverwriteIfNewer OverwriteBehavior = 1
	OverwriteNever   OverwriteBehavior = 2
	OverwriteInvalid OverwriteBehavior = 3
)

// Set implements the method needed for pflag command flag parsing.
func (c *OverwriteBehavior) Set(s string) error {
	switch s {
	case "always":
		*c = OverwriteAlways
	case "if-newer":
		*c = OverwriteIfNewer
	case "never":
		*c = OverwriteNever
	default:
		*c = OverwriteInvalid
		return fmt.Errorf("invalid overwrite behavior %q, must be one of (always|if-newer|never)", s)
	}

	return nil
}

func (c *OverwriteBehavior) String() string {
	switch *c {
	case OverwriteAlways:
		return "always"
	case OverwriteIfNewer:
		return "if-newer"
	case OverwriteNever:
		return "never"
	default:
		return "invalid"
	}

}
func (c *OverwriteBehavior) Type() string {
	return "behavior"
}

// NewRestorer creates a restorer preloaded with the content from the snapshot id.
func NewRestorer(repo restic.Repository, sn *restic.Snapshot, opts Options) *Restorer {
	r := &Restorer{
		repo:         repo,
		sparse:       opts.Sparse,
		progress:     opts.Progress,
		overwrite:    opts.Overwrite,
		fileList:     make(map[string]struct{}),
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
		return err
	}

	res.progress.AddProgress(location, 0, 0)
	return res.restoreNodeMetadataTo(node, target, location)
}

func (res *Restorer) restoreNodeMetadataTo(node *restic.Node, target, location string) error {
	debug.Log("restoreNodeMetadata %v %v %v", node.Name, target, location)
	err := node.RestoreMetadata(target, res.Warn)
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

	res.progress.AddProgress(location, 0, 0)

	// TODO investigate if hardlinks have separate metadata on any supported system
	return res.restoreNodeMetadataTo(node, path, location)
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

	idx := NewHardlinkIndex[string]()
	filerestorer := newFileRestorer(dst, res.repo.LoadBlobsFromPack, res.repo.LookupBlob,
		res.repo.Connections(), res.sparse, res.progress)
	filerestorer.Error = res.Error

	debug.Log("first pass for %q", dst)

	// first tree pass: create directories and collect all files to restore
	_, err = res.traverseTree(ctx, dst, string(filepath.Separator), *res.sn.Tree, treeVisitor{
		enterDir: func(_ *restic.Node, target, location string) error {
			debug.Log("first pass, enterDir: mkdir %q, leaveDir should restore metadata", location)
			res.progress.AddFile(0)
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
				res.progress.AddFile(0)
				return nil
			}

			if node.Links > 1 {
				if idx.Has(node.Inode, node.DeviceID) {
					// a hardlinked file does not increase the restore size
					res.progress.AddFile(0)
					return nil
				}
				idx.Add(node.Inode, node.DeviceID, location)
			}

			return res.withOverwriteCheck(node, target, false, func() error {
				res.progress.AddFile(node.Size)
				filerestorer.addFile(location, node.Content, int64(node.Size))
				res.trackFile(location)
				return nil
			})
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
				return res.withOverwriteCheck(node, target, false, func() error {
					return res.restoreNodeTo(ctx, node, target, location)
				})
			}

			if idx.Has(node.Inode, node.DeviceID) && idx.Value(node.Inode, node.DeviceID) != location {
				return res.withOverwriteCheck(node, target, true, func() error {
					return res.restoreHardlinkAt(node, filerestorer.targetPath(idx.Value(node.Inode, node.DeviceID)), target, location)
				})
			}

			if res.hasRestoredFile(location) {
				return res.restoreNodeMetadataTo(node, target, location)
			}
			// don't touch skipped files
			return nil
		},
		leaveDir: func(node *restic.Node, target, location string) error {
			err := res.restoreNodeMetadataTo(node, target, location)
			if err == nil {
				res.progress.AddProgress(location, 0, 0)
			}
			return err
		},
	})
	return err
}

func (res *Restorer) trackFile(location string) {
	res.fileList[location] = struct{}{}
}

func (res *Restorer) hasRestoredFile(location string) bool {
	_, ok := res.fileList[location]
	return ok
}

func (res *Restorer) withOverwriteCheck(node *restic.Node, target string, isHardlink bool, cb func() error) error {
	overwrite, err := shouldOverwrite(res.overwrite, node, target)
	if err != nil {
		return err
	} else if !overwrite {
		size := node.Size
		if isHardlink {
			size = 0
		}
		res.progress.AddSkippedFile(size)
		return nil
	}
	return cb()
}

func shouldOverwrite(overwrite OverwriteBehavior, node *restic.Node, destination string) (bool, error) {
	if overwrite == OverwriteAlways {
		return true, nil
	}

	fi, err := fs.Lstat(destination)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, err
	}

	if overwrite == OverwriteIfNewer {
		// return if node is newer
		return node.ModTime.After(fi.ModTime()), nil
	} else if overwrite == OverwriteNever {
		// file exists
		return false, nil
	}
	panic("unknown overwrite behavior")
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
				if node.Type != "file" || !res.hasRestoredFile(location) {
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
		length, found := res.repo.LookupBlobSize(restic.DataBlob, blobID)
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
