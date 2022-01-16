package archiver

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"runtime"
	"sort"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	tomb "gopkg.in/tomb.v2"
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
type ErrorFunc func(file string, fi os.FileInfo, err error) error

// ItemStats collects some statistics about a particular file or directory.
type ItemStats struct {
	DataBlobs int    // number of new data blobs added for this item
	DataSize  uint64 // sum of the sizes of all new data blobs
	TreeBlobs int    // number of new tree blobs added for this item
	TreeSize  uint64 // sum of the sizes of all new tree blobs
}

// Add adds other to the current ItemStats.
func (s *ItemStats) Add(other ItemStats) {
	s.DataBlobs += other.DataBlobs
	s.DataSize += other.DataSize
	s.TreeBlobs += other.TreeBlobs
	s.TreeSize += other.TreeSize
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
	// CompleteItem may be called asynchronously from several different
	// goroutines!
	CompleteItem func(item string, previous, current *restic.Node, s ItemStats, d time.Duration)

	// StartFile is called when a file is being processed by a worker.
	StartFile func(filename string)

	// CompleteBlob is called for all saved blobs for files.
	CompleteBlob func(filename string, bytes uint64)

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
	// FileReadConcurrency sets how many files are read in concurrently. If
	// it's set to zero, at most two files are read in concurrently (which
	// turned out to be a good default for most situations).
	FileReadConcurrency uint

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
	if o.FileReadConcurrency == 0 {
		// two is a sweet spot for almost all situations. We've done some
		// experiments documented here:
		// https://github.com/borgbackup/borg/issues/3500
		o.FileReadConcurrency = 2
	}

	if o.SaveBlobConcurrency == 0 {
		o.SaveBlobConcurrency = uint(runtime.NumCPU())
	}

	if o.SaveTreeConcurrency == 0 {
		// use a relatively high concurrency here, having multiple SaveTree
		// workers is cheap
		o.SaveTreeConcurrency = o.SaveBlobConcurrency * 20
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
		CompleteBlob: func(string, uint64) {},
	}

	return arch
}

// error calls arch.Error if it is set and the error is different from context.Canceled.
func (arch *Archiver) error(item string, fi os.FileInfo, err error) error {
	if arch.Error == nil || err == nil {
		return err
	}

	if err == context.Canceled {
		return err
	}

	errf := arch.Error(item, fi, err)
	if err != errf {
		debug.Log("item %v: error was filtered by handler, before: %q, after: %v", item, err, errf)
	}
	return errf
}

// saveTree stores a tree in the repo. It checks the index and the known blobs
// before saving anything.
func (arch *Archiver) saveTree(ctx context.Context, t *restic.Tree) (restic.ID, ItemStats, error) {
	var s ItemStats
	buf, err := json.Marshal(t)
	if err != nil {
		return restic.ID{}, s, errors.Wrap(err, "MarshalJSON")
	}

	// append a newline so that the data is always consistent (json.Encoder
	// adds a newline after each object)
	buf = append(buf, '\n')

	b := &Buffer{Data: buf}
	res := arch.blobSaver.Save(ctx, restic.TreeBlob, b)

	res.Wait(ctx)
	if !res.Known() {
		s.TreeBlobs++
		s.TreeSize += uint64(len(buf))
	}
	// The context was canceled in the meantime, res.ID() might be invalid
	if ctx.Err() != nil {
		return restic.ID{}, s, ctx.Err()
	}
	return res.ID(), s, nil
}

// nodeFromFileInfo returns the restic node from an os.FileInfo.
func (arch *Archiver) nodeFromFileInfo(filename string, fi os.FileInfo) (*restic.Node, error) {
	node, err := restic.NodeFromFileInfo(filename, fi)
	if !arch.WithAtime {
		node.AccessTime = node.ModTime
	}
	return node, errors.Wrap(err, "NodeFromFileInfo")
}

// loadSubtree tries to load the subtree referenced by node. In case of an error, nil is returned.
// If there is no node to load, then nil is returned without an error.
func (arch *Archiver) loadSubtree(ctx context.Context, node *restic.Node) (*restic.Tree, error) {
	if node == nil || node.Type != "dir" || node.Subtree == nil {
		return nil, nil
	}

	tree, err := arch.Repo.LoadTree(ctx, *node.Subtree)
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
func (arch *Archiver) SaveDir(ctx context.Context, snPath string, fi os.FileInfo, dir string, previous *restic.Tree, complete CompleteFunc) (d FutureTree, err error) {
	debug.Log("%v %v", snPath, dir)

	treeNode, err := arch.nodeFromFileInfo(dir, fi)
	if err != nil {
		return FutureTree{}, err
	}

	names, err := readdirnames(arch.FS, dir, fs.O_NOFOLLOW)
	if err != nil {
		return FutureTree{}, err
	}
	sort.Strings(names)

	nodes := make([]FutureNode, 0, len(names))

	for _, name := range names {
		// test if context has been cancelled
		if ctx.Err() != nil {
			debug.Log("context has been cancelled, aborting")
			return FutureTree{}, ctx.Err()
		}

		pathname := arch.FS.Join(dir, name)
		oldNode := previous.Find(name)
		snItem := join(snPath, name)
		fn, excluded, err := arch.Save(ctx, snItem, pathname, oldNode)

		// return error early if possible
		if err != nil {
			err = arch.error(pathname, fi, err)
			if err == nil {
				// ignore error
				continue
			}

			return FutureTree{}, err
		}

		if excluded {
			continue
		}

		nodes = append(nodes, fn)
	}

	ft := arch.treeSaver.Save(ctx, snPath, treeNode, nodes, complete)

	return ft, nil
}

// FutureNode holds a reference to a node, FutureFile, or FutureTree.
type FutureNode struct {
	snPath, target string

	// kept to call the error callback function
	absTarget string
	fi        os.FileInfo

	node  *restic.Node
	stats ItemStats
	err   error

	isFile bool
	file   FutureFile
	isTree bool
	tree   FutureTree
}

func (fn *FutureNode) wait(ctx context.Context) {
	switch {
	case fn.isFile:
		// wait for and collect the data for the file
		fn.file.Wait(ctx)
		fn.node = fn.file.Node()
		fn.err = fn.file.Err()
		fn.stats = fn.file.Stats()

		// ensure the other stuff can be garbage-collected
		fn.file = FutureFile{}
		fn.isFile = false

	case fn.isTree:
		// wait for and collect the data for the dir
		fn.tree.Wait(ctx)
		fn.node = fn.tree.Node()
		fn.stats = fn.tree.Stats()

		// ensure the other stuff can be garbage-collected
		fn.tree = FutureTree{}
		fn.isTree = false
	}
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

	fn = FutureNode{
		snPath: snPath,
		target: target,
	}

	debug.Log("%v target %q, previous %v", snPath, target, previous)
	abstarget, err := arch.FS.Abs(target)
	if err != nil {
		return FutureNode{}, false, err
	}

	fn.absTarget = abstarget

	// exclude files by path before running Lstat to reduce number of lstat calls
	if !arch.SelectByName(abstarget) {
		debug.Log("%v is excluded by path", target)
		return FutureNode{}, true, nil
	}

	// get file info and run remaining select functions that require file information
	fi, err := arch.FS.Lstat(target)
	if err != nil {
		debug.Log("lstat() for %v returned error: %v", target, err)
		err = arch.error(abstarget, fi, err)
		if err != nil {
			return FutureNode{}, false, errors.Wrap(err, "Lstat")
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
		start := time.Now()

		// check if the file has not changed before performing a fopen operation (more expensive, specially
		// in network filesystems)
		if previous != nil && !fileChanged(fi, previous, arch.ChangeIgnoreFlags) {
			if arch.allBlobsPresent(previous) {
				debug.Log("%v hasn't changed, using old list of blobs", target)
				arch.CompleteItem(snPath, previous, previous, ItemStats{}, time.Since(start))
				arch.CompleteBlob(snPath, previous.Size)
				fn.node, err = arch.nodeFromFileInfo(target, fi)
				if err != nil {
					return FutureNode{}, false, err
				}

				// copy list of blobs
				fn.node.Content = previous.Content

				return fn, false, nil
			}

			debug.Log("%v hasn't changed, but contents are missing!", target)
			// There are contents missing - inform user!
			err := errors.Errorf("parts of %v not found in the repository index; storing the file again", target)
			err = arch.error(abstarget, fi, err)
			if err != nil {
				return FutureNode{}, false, err
			}
		}

		// reopen file and do an fstat() on the open file to check it is still
		// a file (and has not been exchanged for e.g. a symlink)
		file, err := arch.FS.OpenFile(target, fs.O_RDONLY|fs.O_NOFOLLOW, 0)
		if err != nil {
			debug.Log("Openfile() for %v returned error: %v", target, err)
			err = arch.error(abstarget, fi, err)
			if err != nil {
				return FutureNode{}, false, errors.Wrap(err, "Lstat")
			}
			return FutureNode{}, true, nil
		}

		fi, err = file.Stat()
		if err != nil {
			debug.Log("stat() on opened file %v returned error: %v", target, err)
			_ = file.Close()
			err = arch.error(abstarget, fi, err)
			if err != nil {
				return FutureNode{}, false, errors.Wrap(err, "Lstat")
			}
			return FutureNode{}, true, nil
		}

		// make sure it's still a file
		if !fs.IsRegularFile(fi) {
			err = errors.Errorf("file %v changed type, refusing to archive")
			_ = file.Close()
			err = arch.error(abstarget, fi, err)
			if err != nil {
				return FutureNode{}, false, err
			}
			return FutureNode{}, true, nil
		}

		fn.isFile = true
		// Save will close the file, we don't need to do that
		fn.file = arch.fileSaver.Save(ctx, snPath, file, fi, func() {
			arch.StartFile(snPath)
		}, func(node *restic.Node, stats ItemStats) {
			arch.CompleteItem(snPath, previous, node, stats, time.Since(start))
		})

	case fi.IsDir():
		debug.Log("  %v dir", target)

		snItem := snPath + "/"
		start := time.Now()
		oldSubtree, err := arch.loadSubtree(ctx, previous)
		if err != nil {
			err = arch.error(abstarget, fi, err)
		}
		if err != nil {
			return FutureNode{}, false, err
		}

		fn.isTree = true
		fn.tree, err = arch.SaveDir(ctx, snPath, fi, target, oldSubtree,
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

		fn.node, err = arch.nodeFromFileInfo(target, fi)
		if err != nil {
			return FutureNode{}, false, err
		}
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
		return nil, errors.Wrap(err, "Lstat")
	}

	tpe := fi.Mode() & (os.ModeType | os.ModeCharDevice)
	if tpe != os.ModeDir {
		return fi, errors.Errorf("path is not a directory: %v", dir)
	}

	return fi, nil
}

// SaveTree stores a Tree in the repo, returned is the tree. snPath is the path
// within the current snapshot.
func (arch *Archiver) SaveTree(ctx context.Context, snPath string, atree *Tree, previous *restic.Tree) (*restic.Tree, error) {
	debug.Log("%v (%v nodes), parent %v", snPath, len(atree.Nodes), previous)

	nodeNames := atree.NodeNames()
	tree := restic.NewTree(len(nodeNames))

	futureNodes := make(map[string]FutureNode)

	// iterate over the nodes of atree in lexicographic (=deterministic) order
	for _, name := range nodeNames {
		subatree := atree.Nodes[name]

		// test if context has been cancelled
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// this is a leaf node
		if subatree.Leaf() {
			fn, excluded, err := arch.Save(ctx, join(snPath, name), subatree.Path, previous.Find(name))

			if err != nil {
				err = arch.error(subatree.Path, fn.fi, err)
				if err == nil {
					// ignore error
					continue
				}
				return nil, err
			}

			if err != nil {
				return nil, err
			}

			if !excluded {
				futureNodes[name] = fn
			}
			continue
		}

		snItem := join(snPath, name) + "/"
		start := time.Now()

		oldNode := previous.Find(name)
		oldSubtree, err := arch.loadSubtree(ctx, oldNode)
		if err != nil {
			err = arch.error(join(snPath, name), nil, err)
		}
		if err != nil {
			return nil, err
		}

		// not a leaf node, archive subtree
		subtree, err := arch.SaveTree(ctx, join(snPath, name), &subatree, oldSubtree)
		if err != nil {
			return nil, err
		}

		id, nodeStats, err := arch.saveTree(ctx, subtree)
		if err != nil {
			return nil, err
		}

		if subatree.FileInfoPath == "" {
			return nil, errors.Errorf("FileInfoPath for %v/%v is empty", snPath, name)
		}

		debug.Log("%v, saved subtree %v as %v", snPath, subtree, id.Str())

		fi, err := arch.statDir(subatree.FileInfoPath)
		if err != nil {
			return nil, err
		}

		debug.Log("%v, dir node data loaded from %v", snPath, subatree.FileInfoPath)

		node, err := arch.nodeFromFileInfo(subatree.FileInfoPath, fi)
		if err != nil {
			return nil, err
		}

		node.Name = name
		node.Subtree = &id

		err = tree.Insert(node)
		if err != nil {
			return nil, err
		}

		arch.CompleteItem(snItem, oldNode, node, nodeStats, time.Since(start))
	}

	debug.Log("waiting on %d nodes", len(futureNodes))

	// process all futures
	for name, fn := range futureNodes {
		fn.wait(ctx)

		// return the error, or ignore it
		if fn.err != nil {
			fn.err = arch.error(fn.target, fn.fi, fn.err)
			if fn.err == nil {
				// ignore error
				continue
			}

			return nil, fn.err
		}

		// when the error is ignored, the node could not be saved, so ignore it
		if fn.node == nil {
			debug.Log("%v excluded: %v", fn.snPath, fn.target)
			continue
		}

		fn.node.Name = name

		err := tree.Insert(fn.node)
		if err != nil {
			return nil, err
		}
	}

	return tree, nil
}

// flags are passed to fs.OpenFile. O_RDONLY is implied.
func readdirnames(filesystem fs.FS, dir string, flags int) ([]string, error) {
	f, err := filesystem.OpenFile(dir, fs.O_RDONLY|flags, 0)
	if err != nil {
		return nil, errors.Wrap(err, "Open")
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
	Paths          []string
	Hostname       string
	Excludes       []string
	Time           time.Time
	ParentSnapshot restic.ID
}

// loadParentTree loads a tree referenced by snapshot id. If id is null, nil is returned.
func (arch *Archiver) loadParentTree(ctx context.Context, snapshotID restic.ID) *restic.Tree {
	if snapshotID.IsNull() {
		return nil
	}

	debug.Log("load parent snapshot %v", snapshotID)
	sn, err := restic.LoadSnapshot(ctx, arch.Repo, snapshotID)
	if err != nil {
		debug.Log("unable to load snapshot %v: %v", snapshotID, err)
		return nil
	}

	if sn.Tree == nil {
		debug.Log("snapshot %v has empty tree %v", snapshotID)
		return nil
	}

	debug.Log("load parent tree %v", *sn.Tree)
	tree, err := arch.Repo.LoadTree(ctx, *sn.Tree)
	if err != nil {
		debug.Log("unable to load tree %v: %v", *sn.Tree, err)
		_ = arch.error("/", nil, arch.wrapLoadTreeError(*sn.Tree, err))
		return nil
	}
	return tree
}

// runWorkers starts the worker pools, which are stopped when the context is cancelled.
func (arch *Archiver) runWorkers(ctx context.Context, t *tomb.Tomb) {
	arch.blobSaver = NewBlobSaver(ctx, t, arch.Repo, arch.Options.SaveBlobConcurrency)

	arch.fileSaver = NewFileSaver(ctx, t,
		arch.blobSaver.Save,
		arch.Repo.Config().ChunkerPolynomial,
		arch.Options.FileReadConcurrency, arch.Options.SaveBlobConcurrency)
	arch.fileSaver.CompleteBlob = arch.CompleteBlob
	arch.fileSaver.NodeFromFileInfo = arch.nodeFromFileInfo

	arch.treeSaver = NewTreeSaver(ctx, t, arch.Options.SaveTreeConcurrency, arch.saveTree, arch.Error)
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

	var t tomb.Tomb
	wctx := t.Context(ctx)
	start := time.Now()

	var rootTreeID restic.ID
	var stats ItemStats
	t.Go(func() error {
		arch.runWorkers(wctx, &t)

		debug.Log("starting snapshot")
		tree, err := arch.SaveTree(wctx, "/", atree, arch.loadParentTree(wctx, opts.ParentSnapshot))
		if err != nil {
			return err
		}

		if len(tree.Nodes) == 0 {
			return errors.New("snapshot is empty")
		}

		rootTreeID, stats, err = arch.saveTree(wctx, tree)
		// trigger shutdown but don't set an error
		t.Kill(nil)
		return err
	})

	err = t.Wait()
	debug.Log("err is %v", err)

	if err != nil {
		debug.Log("error while saving tree: %v", err)
		return nil, restic.ID{}, err
	}

	arch.CompleteItem("/", nil, nil, stats, time.Since(start))

	err = arch.Repo.Flush(ctx)
	if err != nil {
		return nil, restic.ID{}, err
	}

	sn, err := restic.NewSnapshot(opts.Paths, opts.Tags, opts.Hostname, opts.Time)
	if err != nil {
		return nil, restic.ID{}, err
	}

	sn.Excludes = opts.Excludes
	if !opts.ParentSnapshot.IsNull() {
		id := opts.ParentSnapshot
		sn.Parent = &id
	}
	sn.Tree = &rootTreeID

	id, err := arch.Repo.SaveJSONUnpacked(ctx, restic.SnapshotFile, sn)
	if err != nil {
		return nil, restic.ID{}, err
	}

	return sn, id, nil
}
