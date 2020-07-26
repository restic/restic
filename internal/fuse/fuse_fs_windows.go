package fuse

import (
	"strings"
	"sync"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/net/context"
)

const (
	filename = "hello"
	contents = "hello, world\n"
)

func splitPath(path string) []string {
	split := strings.Split(path, "/")

	var emptyFiltered []string
	for _, str := range split {
		if str != "" {
			emptyFiltered = append(emptyFiltered, str)
		}
	}

	return emptyFiltered
}

var defaultDirectoryStat = fuse.Stat_t{
	Mode: fuse.S_IFDIR | 0555,
}

type FsListItemCallback = func(name string, stat *fuse.Stat_t, ofst int64) bool

type FsNode interface {
	Readdir(path []string, callback FsListItemCallback)
	GetAttributes(path []string, stat *fuse.Stat_t) bool
	Open(path []string, flags int) (errc int, fh uint64)
}

type FsNodeRoot struct {
	ctx             context.Context
	repo            restic.Repository
	cfg             Config
	snapshotManager SnapshotManager
	entries         map[string]FsNode
}

var _ = FsNode(&FsNodeRoot{})

func NewNodeRoot(
	ctx context.Context, repo restic.Repository, cfg Config, snapshotManager SnapshotManager,
) *FsNodeRoot {

	root := &FsNodeRoot{
		ctx:             ctx,
		repo:            repo,
		cfg:             cfg,
		snapshotManager: snapshotManager,
	}

	entries := map[string]FsNode{
		"snapshots": NewSnapshotsDir(ctx, root),
	}
	root.entries = entries

	return root
}

func (self *FsNodeRoot) Readdir(path []string, fill FsListItemCallback) {

	debug.Log("FsNodeRoot: Readdir(%v)", path)

	if len(path) == 0 {
		fill(".", nil, 0)
		fill("..", nil, 0)

		for name, _ := range self.entries {
			fill(name, &defaultDirectoryStat, 0)
		}
	} else {
		if entry, found := self.entries[path[0]]; found {
			entry.Readdir(path[1:], fill)
		}
	}
}

func (self *FsNodeRoot) GetAttributes(path []string, stat *fuse.Stat_t) bool {

	if len(path) == 0 {
		*stat = defaultDirectoryStat
		return true
	}

	if entry, found := self.entries[path[0]]; found {
		return entry.GetAttributes(path[1:], stat)
	}

	return false
}

func (self *FsNodeRoot) Open(path []string, flags int) (errc int, fh uint64) {

	lenPath := len(path)

	if lenPath <= 1 {
		return -fuse.EISDIR, ^uint64(0)
	}

	if entry, found := self.entries[path[0]]; found {
		return entry.Open(path[1:], flags)
	}

	return -fuse.ENOENT, ^uint64(0)
}

type FuseFsWindows struct {
	fuse.FileSystemBase
	lock      sync.Mutex
	ctx       context.Context
	repo      restic.Repository
	config    Config
	rootNode  *FsNodeRoot
	blobCache *blobCache
}

// NewRoot initializes a new root node from a repository.
func NewFuseFsWindows(ctx context.Context, repo restic.Repository, cfg Config) *FuseFsWindows {

	debug.Log("NewFuseFsWindows(), config %v", cfg)

	snapshotManager := NewSnapshotManager(ctx, repo, cfg)
	snapshotManager.updateSnapshots()

	rootNode := NewNodeRoot(ctx, repo, cfg, *snapshotManager)

	fuseFsWindows := &FuseFsWindows{
		ctx:       ctx,
		repo:      repo,
		config:    cfg,
		rootNode:  rootNode,
		blobCache: newBlobCache(blobCacheSize),
	}

	return fuseFsWindows
}

func (self *FuseFsWindows) Open(path string, flags int) (errc int, fh uint64) {

	defer self.synchronize()()

	debug.Log("FuseFsWindows: Open(%v, %v)", path, flags)

	splitPath := splitPath(path)
	return self.rootNode.Open(splitPath, flags)

	// switch path {
	// case "/" + filename:
	// 	return 0, 0
	// case "/" + filename + "123":
	// 	return 0, 0
	// default:
	// 	return -fuse.ENOENT, ^uint64(0)
	// }
}

func (self *FuseFsWindows) Read(
	path string, buff []byte, ofst int64, fh uint64,
) (n int) {

	defer self.synchronize()()

	endofst := ofst + int64(len(buff))
	if endofst > int64(len(contents)) {
		endofst = int64(len(contents))
	}
	if endofst < ofst {
		return 0
	}
	n = copy(buff, contents[ofst:endofst])
	return
}

func (self *FuseFsWindows) Readdir(
	path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64,
) (errc int) {

	defer self.synchronize()()

	debug.Log("FuseFsWindows: Readdir(%v)", path)

	splitPath := splitPath(path)

	self.rootNode.Readdir(splitPath, fill)

	return 0
}

func (self *FuseFsWindows) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {

	defer self.synchronize()()

	splitPath := splitPath(path)

	debug.Log("FuseFsWindows: Getattr(%v) -> %v", path, splitPath)

	if self.rootNode.GetAttributes(splitPath, stat) {
		return 0
	}

	return -fuse.ENOENT

	// switch path {
	// case "/":
	// 	stat.Mode = fuse.S_IFDIR | 0555
	// 	return 0
	// case "/" + filename:
	// 	stat.Mode = fuse.S_IFREG | 0444
	// 	stat.Size = int64(len(contents))
	// 	return 0
	// case "/" + filename + "123":
	// 	stat.Mode = fuse.S_IFREG | 0444
	// 	stat.Size = int64(len(contents))
	// 	return 0
	// default:
	// 	return -fuse.ENOENT
	// }
}

func (self *FuseFsWindows) synchronize() func() {
	self.lock.Lock()
	return func() {
		self.lock.Unlock()
	}
}
