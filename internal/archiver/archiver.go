package archiver

import (
	"context"
	"os"
	"path"
	"runtime"
	"sort"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

// SelectByNameFunc returns true for all items that should be included (files and
// dirs). If false is returned, files are ignored and dirs are not even walked.
type SelectByNameFunc func(item string) bool

// SelectFunc returns true for all items that should be included (files and
// dirs). If false is returned, files are ignored and dirs are not even walked.
type SelectFunc func(item string, fi os.FileInfo) bool

// ErrorFunc is called when an error during archiving occurs. When nil is
// returned, the archiver continues, otherwise it aborts and passes the error
// up the call stack.
type ErrorFunc func(file string, err error) error

// ItemStats collects some statistics about a particular file or directory.
type ItemStats struct {
	DataBlobs      int    // number of new data blobs added for this item
	DataSize       uint64 // sum of the sizes of all new data blobs
	DataSizeInRepo uint64 // sum of the bytes added to the repo (including compression and crypto overhead)
	TreeBlobs      int    // number of new tree blobs added for this item
	TreeSize       uint64 // sum of the sizes of all new tree blobs
	TreeSizeInRepo uint64 // sum of the bytes added to the repo (including compression and crypto overhead)
}

// Add adds other to the current ItemStats.
func (s *ItemStats) Add(other ItemStats) {
	s.DataBlobs += other.DataBlobs
	s.DataSize += other.DataSize
	s.DataSizeInRepo += other.DataSizeInRepo
	s.TreeBlobs += other.TreeBlobs
	s.TreeSize += other.TreeSize
	s.TreeSizeInRepo += other.TreeSizeInRepo
}

// Archiver saves a directory structure to the repo.
type Archiver struct {
	Repo         restic.Repository
	SelectByName SelectByNameFunc
	Select       SelectFunc
	FS           fs.FS
	Options      Options

	blobSaver *BlobSaver
	fileSaver *FileSaver
	treeSaver *TreeSaver

	// Error is called for all errors that occur during backup.
	Error ErrorFunc

	// CompleteItem is called for all files and dirs once they have been
	// processed successfully. The parameter item contains the path as it will
	// be in the snapshot after saving. s contains some statistics about this
	// particular file/dir.
	//
	// Once reading a file has completed successfully (but not saving it yet),
	// CompleteItem will be called with current == nil.
	//
	// CompleteItem may be called asynchronously from several different
	// goroutines!
	CompleteItem func(item string, previous, current *restic.Node, s ItemStats, d time.Duration)

	// StartFile is called when a file is being processed by a worker.
	StartFile func(filename string)

	// CompleteBlob is called for all saved blobs for files.
	CompleteBlob func(bytes uint64)

	// WithAtime configures if the access time for files and directories should
	// be saved. Enabling it may result in much metadata, so it's off by
	// default.
	WithAtime bool

	// Flags controlling change detection. See doc/040_backup.rst for details.
	ChangeIgnoreFlags uint
}

// Flags for the ChangeIgnoreFlags bitfield.
const (
	ChangeIgnoreCtime = 1 << iota
	ChangeIgnoreInode
)

// Options is used to configure the archiver.
type Options struct {
	// ReadConcurrency sets how many files are read in concurrently. If
	// it's set to zero, at most two files are read in concurrently (which
	// turned out to be a good default for most situations).
	ReadConcurrency uint

	// SaveBlobConcurrency sets how many blobs are hashed and saved
	// concurrently. If it's set to zero, the default is the number of CPUs
	// available in the system.
	SaveBlobConcurrency uint

	// SaveTreeConcurrency sets how many trees are marshalled and saved to the
	// repo concurrently.
	SaveTreeConcurrency uint
}

// ApplyDefaults returns a copy of o with the default options set for all unset
// fields.
func (o Options) ApplyDefaults() Options {
	if o.ReadConcurrency == 0 {
		// two is a sweet spot for almost all situations. We've done some
		// experiments documented here:
		// https://github.com/borgbackup/borg/issues/3500
		o.ReadConcurrency = 2
	}

	if o.SaveBlobConcurrency == 0 {
		// blob saving is CPU bound due to hash checking and encryption
		// the actual upload is handled by the repository itself
		o.SaveBlobConcurrency = uint(runtime.GOMAXPROCS(0))
	}

	if o.SaveTreeConcurrency == 0 {
		// can either wait for a file, wait for a tree, serialize a tree or wait for saveblob
		// the last two are cpu-bound and thus mutually exclusive.
		// Also allow waiting for FileReadConcurrency files, this is the maximum of FutureFiles
		// which currently can be in progress. The main backup loop blocks when trying to queue
		// more files to read.
		o.SaveTreeConcurrency = uint(runtime.GOMAXPROCS(0)) + o.ReadConcurrency
	}

	return o
}

// New initializes a new archiver.
func New(repo restic.Repository, fs fs.FS, opts Options) *Archiver {
	arch := &Archiver{
		Repo:         repo,
		SelectByName: func(item string) bool { return true },
		Select:       func(item string, fi os.FileInfo) bool { return true },
		FS:           fs,
		Options:      opts.ApplyDefaults(),

		CompleteItem: func(string, *restic.Node, *restic.Node, ItemStats, time.Duration) {},
		StartFile:    func(string) {},
		CompleteBlob: func(uint64) {},
	}

	return arch
}

// error calls arch.Error if it is set and the error is different from context.Canceled.
func (arch *Archiver) error(item string, err error) error {
	if arch.Error == nil || err == nil {
		return err
	}

	if err == context.Canceled {
		return err
	}

	errf := arch.Error(item, err)
	if err != errf {
		debug.Log("item %v: error was filtered by handler, before: %q, after: %v", item, err, errf)
	}
	return errf
}

// nodeFromFileInfo returns the restic node from an os.FileInfo.
func (arch *Archiver) nodeFromFileInfo(snPath, filename string, fi os.FileInfo) (*restic.Node, error) {
	node, err := restic.NodeFromFileInfo(filename, fi)
	if !arch.WithAtime {
		node.AccessTime = node.ModTime
	}
	// overwrite name to match that within the snapshot
	node.Name = path.Base(snPath)
	return node, errors.WithStack(err)
}

// loadSubtree tries to load the subtree referenced by node. In case of an error, nil is returned.
// If there is no node to load, then nil is returned without an error.
func (arch *Archiver) loadSubtree(ctx context.Context, node *restic.Node) (*restic.Tree, error) {
	if node == nil || node.Type != "dir" || node.Subtree == nil {
		return nil, nil
	}

	tree, err := restic.LoadTree(ctx, arch.Repo, *node.Subtree)
	if err != nil {
		debug.Log("unable to load tree %v: %v", node.Subtree.Str(), err)
		// a tree in the repository is not readable -> warn the user
		return nil, arch.wrapLoadTreeError(*node.Subtree, err)
	}

	return tree, nil
}

func (arch *Archiver) wrapLoadTreeError(id restic.ID, err error) error {
	if arch.Repo.Index().Has(restic.BlobHandle{ID: id, Type: restic.TreeBlob}) {
		err = errors.Errorf("tree %v could not be loaded; the repository could be damaged: %v", id, err)
	} else {
		err = errors.Errorf("tree %v is not known; the repository could be damaged, run `rebuild-index` to try to repair it", id)
	}
	return err
}

// SaveDir stores a directory in the repo and returns the node. snPath is the
// path within the current snapshot.
func (arch *Archiver) SaveDir(ctx context.Context, snPath string, dir string, fi os.FileInfo, previous *restic.Tree, complete CompleteFunc) (d FutureNode, err error) {
	debug.Log("%v %v", snPath, dir)

	treeNode, err := arch.nodeFromFileInfo(snPath, dir, fi)
	if err != nil {
		return FutureNode{}, err
	}

	names, err := readdirnames(arch.FS, dir, fs.O_NOFOLLOW)
	if err != nil {
		return FutureNode{}, err
	}
	sort.Strings(names)

	nodes := make([]FutureNode, 0, len(names))

	for _, name := range names {
		// test if context has been cancelled
		if ctx.Err() != nil {
			debug.Log("context has been cancelled, aborting")
			return FutureNode{}, ctx.Err()
		}

		pathname := arch.FS.Join(dir, name)
		oldNode := previous.Find(name)
		snItem := join(snPath, name)
		fn, excluded, err := arch.Save(ctx, snItem, pathname, oldNode)

		// return error early if possible
		if err != nil {
			err = arch.error(pathname, err)
			if err == nil {
				// ignore error
				continue
			}

			return FutureNode{}, err
		}

		if excluded {
			continue
		}

		nodes = append(nodes, fn)
	}

	fn := arch.treeSaver.Save(ctx, snPath, dir, treeNode, nodes, complete)

	return fn, nil
}

// FutureNode holds a reference to a channel that returns a FutureNodeResult
// or a reference to an already existing result. If the result is available
// immediatelly, then storing a reference directly requires less memory than
// using the indirection via a channel.
type FutureNode struct {
	ch  <-chan futureNodeResult
	res *futureNodeResult
}

type futureNodeResult struct {
	snPath, target string

	node  *restic.Node
	stats ItemStats
	err   error
}

func newFutureNode() (FutureNode, chan<- futureNodeResult) {
	ch := make(chan futureNodeResult, 1)
	return FutureNode{ch: ch}, ch
}

func newFutureNodeWithResult(res futureNodeResult) FutureNode {
	return FutureNode{
		res: &res,
	}
}

func (fn *FutureNode) take(ctx context.Context) futureNodeResult {
	if fn.res != nil {
		res := fn.res
		// free result
		fn.res = nil
		return *res
	}
	select {
	case res, ok := <-fn.ch:
		if ok {
			// free channel
			fn.ch = nil
			return res
		}
	case <-ctx.Done():
	}
	return futureNodeResult{err: errors.Errorf("no result")}
}

// allBlobsPresent checks if all blobs (contents) of the given node are
// present in the index.
func (arch *Archiver) allBlobsPresent(previous *restic.Node) bool {
	// check if all blobs are contained in index
	for _, id := range previous.Content {
		if !arch.Repo.Index().Has(restic.BlobHandle{ID: id, Type: restic.DataBlob}) {
			return false
		}
	}
	return true
}

// Save saves a target (file or directory) to the repo. If the item is
// excluded, this function returns a nil node and error, with excluded set to
// true.
//
// Errors and completion needs to be handled by the caller.
//
// snPath is the path within the current snapshot.
func (arch *Archiver) Save(ctx context.Context, snPath, target string, previous *restic.Node) (fn FutureNode, excluded bool, err error) {
	start := time.Now()

	debug.Log("%v target %q, previous %v", snPath, target, previous)
	abstarget, err := arch.FS.Abs(target)
	if err != nil {
		return FutureNode{}, false, err
	}

	// exclude files by path before running Lstat to reduce number of lstat calls
	if !arch.SelectByName(abstarget) {
		debug.Log("%v is excluded by path", target)
		return FutureNode{}, true, nil
	}

	// get file info and run remaining select functions that require file information
	fi, err := arch.FS.Lstat(target)
	if err != nil {
		debug.Log("lstat() for %v returned error: %v", target, err)
		err = arch.error(abstarget, err)
		if err != nil {
			return FutureNode{}, false, errors.WithStack(err)
		}
		return FutureNode{}, true, nil
	}
	if !arch.Select(abstarget, fi) {
		debug.Log("%v is excluded", target)
		return FutureNode{}, true, nil
	}

	switch {
	case fs.IsRegularFile(fi):
		debug.Log("  %v regular file", target)

		// check if the file has not changed before performing a fopen operation (more expensive, specially
		// in network filesystems)
		if previous != nil && !fileChanged(fi, previous, arch.ChangeIgnoreFlags) {
			if arch.allBlobsPresent(previous) {
				debug.Log("%v hasn't changed, using old list of blobs", target)
				arch.CompleteItem(snPath, previous, previous, ItemStats{}, time.Since(start))
				arch.CompleteBlob(previous.Size)
				node, err := arch.nodeFromFileInfo(snPath, target, fi)
				if err != nil {
					return FutureNode{}, false, err
				}

				// copy list of blobs
				node.Content = previous.Content

				fn = newFutureNodeWithResult(futureNodeResult{
					snPath: snPath,
					target: target,
					node:   node,
				})
				return fn, false, nil
			}

			debug.Log("%v hasn't changed, but contents are missing!", target)
			// There are contents missing - inform user!
			err := errors.Errorf("parts of %v not found in the repository index; storing the file again", target)
			err = arch.error(abstarget, err)
			if err != nil {
				return FutureNode{}, false, err
			}
		}

		// reopen file and do an fstat() on the open file to check it is still
		// a file (and has not been exchanged for e.g. a symlink)
		file, err := arch.FS.OpenFile(target, fs.O_RDONLY|fs.O_NOFOLLOW, 0)
		if err != nil {
			debug.Log("Openfile() for %v returned error: %v", target, err)
			err = arch.error(abstarget, err)
			if err != nil {
				return FutureNode{}, false, errors.WithStack(err)
			}
			return FutureNode{}, true, nil
		}

		fi, err = file.Stat()
		if err != nil {
			debug.Log("stat() on opened file %v returned error: %v", target, err)
			_ = file.Close()
			err = arch.error(abstarget, err)
			if err != nil {
				return FutureNode{}, false, errors.WithStack(err)
			}
			return FutureNode{}, true, nil
		}

		// make sure it's still a file
		if !fs.IsRegularFile(fi) {
			err = errors.Errorf("file %v changed type, refusing to archive", fi.Name())
			_ = file.Close()
			err = arch.error(abstarget, err)
			if err != nil {
				return FutureNode{}, false, err
			}
			return FutureNode{}, true, nil
		}

		// Save will close the file, we don't need to do that
		fn = arch.fileSaver.Save(ctx, snPath, target, file, fi, func() {
			arch.StartFile(snPath)
		}, func() {
			arch.CompleteItem(snPath, nil, nil, ItemStats{}, 0)
		}, func(node *restic.Node, stats ItemStats) {
			arch.CompleteItem(snPath, previous, node, stats, time.Since(start))
		})

	case fi.IsDir():
		debug.Log("  %v dir", target)

		snItem := snPath + "/"
		oldSubtree, err := arch.loadSubtree(ctx, previous)
		if err != nil {
			err = arch.error(abstarget, err)
		}
		if err != nil {
			return FutureNode{}, false, err
		}

		fn, err = arch.SaveDir(ctx, snPath, target, fi, oldSubtree,
			func(node *restic.Node, stats ItemStats) {
				arch.CompleteItem(snItem, previous, node, stats, time.Since(start))
			})
		if err != nil {
			debug.Log("SaveDir for %v returned error: %v", snPath, err)
			return FutureNode{}, false, err
		}

	case fi.Mode()&os.ModeSocket > 0:
		debug.Log("  %v is a socket, ignoring", target)
		return FutureNode{}, true, nil

	default:
		debug.Log("  %v other", target)

		node, err := arch.nodeFromFileInfo(snPath, target, fi)
		if err != nil {
			return FutureNode{}, false, err
		}
		fn = newFutureNodeWithResult(futureNodeResult{
			snPath: snPath,
			target: target,
			node:   node,
		})
	}

	debug.Log("return after %.3f", time.Since(start).Seconds())

	return fn, false, nil
}

// fileChanged tries to detect whether a file's content has changed compared
// to the contents of node, which describes the same path in the parent backup.
// It should only be run for regular files.
func fileChanged(fi os.FileInfo, node *restic.Node, ignoreFlags uint) bool {
	switch {
	case node == nil:
		return true
	case node.Type != "file":
		// We're only called for regular files, so this is a type change.
		return true
	case uint64(fi.Size()) != node.Size:
		return true
	case !fi.ModTime().Equal(node.ModTime):
		return true
	}

	checkCtime := ignoreFlags&ChangeIgnoreCtime == 0
	checkInode := ignoreFlags&ChangeIgnoreInode == 0

	extFI := fs.ExtendedStat(fi)
	switch {
	case checkCtime && !extFI.ChangeTime.Equal(node.ChangeTime):
		return true
	case checkInode && node.Inode != extFI.Inode:
		return true
	}

	return false
}

// join returns all elements separated with a forward slash.
func join(elem ...string) string {
	return path.Join(elem...)
}

// statDir returns the file info for the directory. Symbolic links are
// resolved. If the target directory is not a directory, an error is returned.
func (arch *Archiver) statDir(dir string) (os.FileInfo, error) {
	fi, err := arch.FS.Stat(dir)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	tpe := fi.Mode() & (os.ModeType | os.ModeCharDevice)
	if tpe != os.ModeDir {
		return fi, errors.Errorf("path is not a directory: %v", dir)
	}

	return fi, nil
}

// SaveTree stores a Tree in the repo, returned is the tree. snPath is the path
// within the current snapshot.
func (arch *Archiver) SaveTree(ctx context.Context, snPath string, atree *Tree, previous *restic.Tree, complete CompleteFunc) (FutureNode, int, error) {

	var node *restic.Node
	if snPath != "/" {
		if atree.FileInfoPath == "" {
			return FutureNode{}, 0, errors.Errorf("FileInfoPath for %v is empty", snPath)
		}

		fi, err := arch.statDir(atree.FileInfoPath)
		if err != nil {
			return FutureNode{}, 0, err
		}

		debug.Log("%v, dir node data loaded from %v", snPath, atree.FileInfoPath)
		node, err = arch.nodeFromFileInfo(snPath, atree.FileInfoPath, fi)
		if err != nil {
			return FutureNode{}, 0, err
		}
	} else {
		// fake root node
		node = &restic.Node{}
	}

	debug.Log("%v (%v nodes), parent %v", snPath, len(atree.Nodes), previous)
	nodeNames := atree.NodeNames()
	nodes := make([]FutureNode, 0, len(nodeNames))

	// iterate over the nodes of atree in lexicographic (=deterministic) order
	for _, name := range nodeNames {
		subatree := atree.Nodes[name]

		// test if context has been cancelled
		if ctx.Err() != nil {
			return FutureNode{}, 0, ctx.Err()
		}

		// this is a leaf node
		if subatree.Leaf() {
			fn, excluded, err := arch.Save(ctx, join(snPath, name), subatree.Path, previous.Find(name))

			if err != nil {
				err = arch.error(subatree.Path, err)
				if err == nil {
					// ignore error
					continue
				}
				return FutureNode{}, 0, err
			}

			if err != nil {
				return FutureNode{}, 0, err
			}

			if !excluded {
				nodes = append(nodes, fn)
			}
			continue
		}

		snItem := join(snPath, name) + "/"
		start := time.Now()

		oldNode := previous.Find(name)
		oldSubtree, err := arch.loadSubtree(ctx, oldNode)
		if err != nil {
			err = arch.error(join(snPath, name), err)
		}
		if err != nil {
			return FutureNode{}, 0, err
		}

		// not a leaf node, archive subtree
		fn, _, err := arch.SaveTree(ctx, join(snPath, name), &subatree, oldSubtree, func(n *restic.Node, is ItemStats) {
			arch.CompleteItem(snItem, oldNode, n, is, time.Since(start))
		})
		if err != nil {
			return FutureNode{}, 0, err
		}
		nodes = append(nodes, fn)
	}

	fn := arch.treeSaver.Save(ctx, snPath, atree.FileInfoPath, node, nodes, complete)
	return fn, len(nodes), nil
}

// flags are passed to fs.OpenFile. O_RDONLY is implied.
func readdirnames(filesystem fs.FS, dir string, flags int) ([]string, error) {
	f, err := filesystem.OpenFile(dir, fs.O_RDONLY|flags, 0)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	entries, err := f.Readdirnames(-1)
	if err != nil {
		_ = f.Close()
		return nil, errors.Wrapf(err, "Readdirnames %v failed", dir)
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// resolveRelativeTargets replaces targets that only contain relative
// directories ("." or "../../") with the contents of the directory. Each
// element of target is processed with fs.Clean().
func resolveRelativeTargets(filesys fs.FS, targets []string) ([]string, error) {
	debug.Log("targets before resolving: %v", targets)
	result := make([]string, 0, len(targets))
	for _, target := range targets {
		target = filesys.Clean(target)
		pc, _ := pathComponents(filesys, target, false)
		if len(pc) > 0 {
			result = append(result, target)
			continue
		}

		debug.Log("replacing %q with readdir(%q)", target, target)
		entries, err := readdirnames(filesys, target, fs.O_NOFOLLOW)
		if err != nil {
			return nil, err
		}
		sort.Strings(entries)

		for _, name := range entries {
			result = append(result, filesys.Join(target, name))
		}
	}

	debug.Log("targets after resolving: %v", result)
	return result, nil
}

// SnapshotOptions collect attributes for a new snapshot.
type SnapshotOptions struct {
	Tags           restic.TagList
	Hostname       string
	Excludes       []string
	Time           time.Time
	ParentSnapshot *restic.Snapshot
}

// loadParentTree loads a tree referenced by snapshot id. If id is null, nil is returned.
func (arch *Archiver) loadParentTree(ctx context.Context, sn *restic.Snapshot) *restic.Tree {
	if sn == nil {
		return nil
	}

	if sn.Tree == nil {
		debug.Log("snapshot %v has empty tree %v", *sn.ID())
		return nil
	}

	debug.Log("load parent tree %v", *sn.Tree)
	tree, err := restic.LoadTree(ctx, arch.Repo, *sn.Tree)
	if err != nil {
		debug.Log("unable to load tree %v: %v", *sn.Tree, err)
		_ = arch.error("/", arch.wrapLoadTreeError(*sn.Tree, err))
		return nil
	}
	return tree
}

// runWorkers starts the worker pools, which are stopped when the context is cancelled.
func (arch *Archiver) runWorkers(ctx context.Context, wg *errgroup.Group) {
	arch.blobSaver = NewBlobSaver(ctx, wg, arch.Repo, arch.Options.SaveBlobConcurrency)

	arch.fileSaver = NewFileSaver(ctx, wg,
		arch.blobSaver.Save,
		arch.Repo.Config().ChunkerPolynomial,
		arch.Options.ReadConcurrency, arch.Options.SaveBlobConcurrency)
	arch.fileSaver.CompleteBlob = arch.CompleteBlob
	arch.fileSaver.NodeFromFileInfo = arch.nodeFromFileInfo

	arch.treeSaver = NewTreeSaver(ctx, wg, arch.Options.SaveTreeConcurrency, arch.blobSaver.Save, arch.Error)
}

func (arch *Archiver) stopWorkers() {
	arch.blobSaver.TriggerShutdown()
	arch.fileSaver.TriggerShutdown()
	arch.treeSaver.TriggerShutdown()
	arch.blobSaver = nil
	arch.fileSaver = nil
	arch.treeSaver = nil
}

// Snapshot saves several targets and returns a snapshot.
func (arch *Archiver) Snapshot(ctx context.Context, targets []string, opts SnapshotOptions) (*restic.Snapshot, restic.ID, error) {
	cleanTargets, err := resolveRelativeTargets(arch.FS, targets)
	if err != nil {
		return nil, restic.ID{}, err
	}

	atree, err := NewTree(arch.FS, cleanTargets)
	if err != nil {
		return nil, restic.ID{}, err
	}

	var rootTreeID restic.ID

	wgUp, wgUpCtx := errgroup.WithContext(ctx)
	arch.Repo.StartPackUploader(wgUpCtx, wgUp)

	wgUp.Go(func() error {
		wg, wgCtx := errgroup.WithContext(wgUpCtx)
		start := time.Now()

		wg.Go(func() error {
			arch.runWorkers(wgCtx, wg)

			debug.Log("starting snapshot")
			fn, nodeCount, err := arch.SaveTree(wgCtx, "/", atree, arch.loadParentTree(wgCtx, opts.ParentSnapshot), func(n *restic.Node, is ItemStats) {
				arch.CompleteItem("/", nil, nil, is, time.Since(start))
			})
			if err != nil {
				return err
			}

			fnr := fn.take(wgCtx)
			if fnr.err != nil {
				return fnr.err
			}

			if wgCtx.Err() != nil {
				return wgCtx.Err()
			}

			if nodeCount == 0 {
				return errors.New("snapshot is empty")
			}

			rootTreeID = *fnr.node.Subtree
			arch.stopWorkers()
			return nil
		})

		err = wg.Wait()
		debug.Log("err is %v", err)

		if err != nil {
			debug.Log("error while saving tree: %v", err)
			return err
		}

		return arch.Repo.Flush(ctx)
	})
	err = wgUp.Wait()
	if err != nil {
		return nil, restic.ID{}, err
	}

	sn, err := restic.NewSnapshot(targets, opts.Tags, opts.Hostname, opts.Time)
	if err != nil {
		return nil, restic.ID{}, err
	}

	sn.Excludes = opts.Excludes
	if opts.ParentSnapshot != nil {
		sn.Parent = opts.ParentSnapshot.ID()
	}
	sn.Tree = &rootTreeID

	id, err := restic.SaveSnapshot(ctx, arch.Repo, sn)
	if err != nil {
		return nil, restic.ID{}, err
	}

	return sn, id, nil
}
