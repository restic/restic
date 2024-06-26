package restorer

import (
	"context"
	"fmt"
	"io"
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
	repo restic.Repository
	sn   *restic.Snapshot
	opts Options

	fileList map[string]bool

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
	OverwriteAlways OverwriteBehavior = iota
	// OverwriteIfChanged is like OverwriteAlways except that it skips restoring the content
	// of files with matching size&mtime. Metatdata is always restored.
	OverwriteIfChanged
	OverwriteIfNewer
	OverwriteNever
	OverwriteInvalid
)

// Set implements the method needed for pflag command flag parsing.
func (c *OverwriteBehavior) Set(s string) error {
	switch s {
	case "always":
		*c = OverwriteAlways
	case "if-changed":
		*c = OverwriteIfChanged
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
	case OverwriteIfChanged:
		return "if-changed"
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
		opts:         opts,
		fileList:     make(map[string]bool),
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
	if err := fs.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Wrap(err, "RemoveNode")
	}

	err := node.CreateAt(ctx, target, res.repo)
	if err != nil {
		debug.Log("node.CreateAt(%s) error %v", target, err)
		return err
	}

	res.opts.Progress.AddProgress(location, 0, 0)
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
	if err := fs.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Wrap(err, "RemoveCreateHardlink")
	}
	err := fs.Link(target, path)
	if err != nil {
		return errors.WithStack(err)
	}

	res.opts.Progress.AddProgress(location, 0, 0)

	// TODO investigate if hardlinks have separate metadata on any supported system
	return res.restoreNodeMetadataTo(node, path, location)
}

func (res *Restorer) ensureDir(target string) error {
	fi, err := fs.Lstat(target)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check for directory: %w", err)
	}
	if err == nil && !fi.IsDir() {
		// try to cleanup unexpected file
		if err := fs.Remove(target); err != nil {
			return fmt.Errorf("failed to remove stale item: %w", err)
		}
	}

	// create parent dir with default permissions
	// second pass #leaveDir restores dir metadata after visiting/restoring all children
	return fs.MkdirAll(target, 0700)
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
		res.repo.Connections(), res.opts.Sparse, res.opts.Progress)
	filerestorer.Error = res.Error

	debug.Log("first pass for %q", dst)

	var buf []byte

	// first tree pass: create directories and collect all files to restore
	_, err = res.traverseTree(ctx, dst, string(filepath.Separator), *res.sn.Tree, treeVisitor{
		enterDir: func(_ *restic.Node, target, location string) error {
			debug.Log("first pass, enterDir: mkdir %q, leaveDir should restore metadata", location)
			res.opts.Progress.AddFile(0)
			return res.ensureDir(target)
		},

		visitNode: func(node *restic.Node, target, location string) error {
			debug.Log("first pass, visitNode: mkdir %q, leaveDir on second pass should restore metadata", location)
			if err := res.ensureDir(filepath.Dir(target)); err != nil {
				return err
			}

			if node.Type != "file" {
				res.opts.Progress.AddFile(0)
				return nil
			}

			if node.Links > 1 {
				if idx.Has(node.Inode, node.DeviceID) {
					// a hardlinked file does not increase the restore size
					res.opts.Progress.AddFile(0)
					return nil
				}
				idx.Add(node.Inode, node.DeviceID, location)
			}

			buf, err = res.withOverwriteCheck(node, target, false, buf, func(updateMetadataOnly bool, matches *fileState) error {
				if updateMetadataOnly {
					res.opts.Progress.AddSkippedFile(node.Size)
				} else {
					res.opts.Progress.AddFile(node.Size)
					filerestorer.addFile(location, node.Content, int64(node.Size), matches)
				}
				res.trackFile(location, updateMetadataOnly)
				return nil
			})
			return err
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
				_, err := res.withOverwriteCheck(node, target, false, nil, func(_ bool, _ *fileState) error {
					return res.restoreNodeTo(ctx, node, target, location)
				})
				return err
			}

			if idx.Has(node.Inode, node.DeviceID) && idx.Value(node.Inode, node.DeviceID) != location {
				_, err := res.withOverwriteCheck(node, target, true, nil, func(_ bool, _ *fileState) error {
					return res.restoreHardlinkAt(node, filerestorer.targetPath(idx.Value(node.Inode, node.DeviceID)), target, location)
				})
				return err
			}

			if _, ok := res.hasRestoredFile(location); ok {
				return res.restoreNodeMetadataTo(node, target, location)
			}
			// don't touch skipped files
			return nil
		},
		leaveDir: func(node *restic.Node, target, location string) error {
			err := res.restoreNodeMetadataTo(node, target, location)
			if err == nil {
				res.opts.Progress.AddProgress(location, 0, 0)
			}
			return err
		},
	})
	return err
}

func (res *Restorer) trackFile(location string, metadataOnly bool) {
	res.fileList[location] = metadataOnly
}

func (res *Restorer) hasRestoredFile(location string) (metadataOnly bool, ok bool) {
	metadataOnly, ok = res.fileList[location]
	return metadataOnly, ok
}

func (res *Restorer) withOverwriteCheck(node *restic.Node, target string, isHardlink bool, buf []byte, cb func(updateMetadataOnly bool, matches *fileState) error) ([]byte, error) {
	overwrite, err := shouldOverwrite(res.opts.Overwrite, node, target)
	if err != nil {
		return buf, err
	} else if !overwrite {
		size := node.Size
		if isHardlink {
			size = 0
		}
		res.opts.Progress.AddSkippedFile(size)
		return buf, nil
	}

	var matches *fileState
	updateMetadataOnly := false
	if node.Type == "file" && !isHardlink {
		// if a file fails to verify, then matches is nil which results in restoring from scratch
		matches, buf, _ = res.verifyFile(target, node, false, res.opts.Overwrite == OverwriteIfChanged, buf)
		// skip files that are already correct completely
		updateMetadataOnly = !matches.NeedsRestore()
	}

	return buf, cb(updateMetadataOnly, matches)
}

func shouldOverwrite(overwrite OverwriteBehavior, node *restic.Node, destination string) (bool, error) {
	if overwrite == OverwriteAlways || overwrite == OverwriteIfChanged {
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
				if node.Type != "file" {
					return nil
				}
				if metadataOnly, ok := res.hasRestoredFile(location); !ok || metadataOnly {
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
				_, buf, err = res.verifyFile(job.path, job.node, true, false, buf)
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

type fileState struct {
	blobMatches []bool
	sizeMatches bool
}

func (s *fileState) NeedsRestore() bool {
	if s == nil {
		return true
	}
	if !s.sizeMatches {
		return true
	}
	for _, match := range s.blobMatches {
		if !match {
			return true
		}
	}
	return false
}

func (s *fileState) HasMatchingBlob(i int) bool {
	if s == nil || s.blobMatches == nil {
		return false
	}
	return i < len(s.blobMatches) && s.blobMatches[i]
}

// Verify that the file target has the contents of node.
//
// buf and the first return value are scratch space, passed around for reuse.
// Reusing buffers prevents the verifier goroutines allocating all of RAM and
// flushing the filesystem cache (at least on Linux).
func (res *Restorer) verifyFile(target string, node *restic.Node, failFast bool, trustMtime bool, buf []byte) (*fileState, []byte, error) {
	f, err := fs.OpenFile(target, fs.O_RDONLY|fs.O_NOFOLLOW, 0)
	if err != nil {
		return nil, buf, err
	}
	defer func() {
		_ = f.Close()
	}()

	fi, err := f.Stat()
	sizeMatches := true
	switch {
	case err != nil:
		return nil, buf, err
	case !fi.Mode().IsRegular():
		return nil, buf, errors.Errorf("Expected %s to be a regular file", target)
	case int64(node.Size) != fi.Size():
		if failFast {
			return nil, buf, errors.Errorf("Invalid file size for %s: expected %d, got %d",
				target, node.Size, fi.Size())
		}
		sizeMatches = false
	}

	if trustMtime && fi.ModTime().Equal(node.ModTime) && sizeMatches {
		return &fileState{nil, sizeMatches}, buf, nil
	}

	matches := make([]bool, len(node.Content))
	var offset int64
	for i, blobID := range node.Content {
		length, found := res.repo.LookupBlobSize(restic.DataBlob, blobID)
		if !found {
			return nil, buf, errors.Errorf("Unable to fetch blob %s", blobID)
		}

		if length > uint(cap(buf)) {
			buf = make([]byte, 2*length)
		}
		buf = buf[:length]

		_, err = f.ReadAt(buf, offset)
		if err == io.EOF && !failFast {
			sizeMatches = false
			break
		}
		if err != nil {
			return nil, buf, err
		}
		matches[i] = blobID.Equal(restic.Hash(buf))
		if failFast && !matches[i] {
			return nil, buf, errors.Errorf(
				"Unexpected content in %s, starting at offset %d",
				target, offset)
		}
		offset += int64(length)
	}

	return &fileState{matches, sizeMatches}, buf, nil
}
