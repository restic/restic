package fuse

import (
	"strings"
	"sync"

	"github.com/billziss-gh/cgofuse/fuse"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/net/context"
)

// splitPath splits the path received from the fuse library to an array for
// easier handling when traversing nodes
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

// defaultDirectoryStat are the default directory stats for all virtual
// directories.
var defaultDirectoryStat = fuse.Stat_t{
	Mode: fuse.S_IFDIR | 0555,
}

// FsListItemCallback function is used to return directory information from
// Readdir().
type FsListItemCallback = func(name string, stat *fuse.Stat_t, ofst int64) bool

// FsNode the interface for all nodes in the virtual filesystem.
type FsNode interface {
	Readdir(path []string, callback FsListItemCallback)
	GetAttributes(path []string, stat *fuse.Stat_t) bool
	Open(path []string, flags int) (errc int, fh uint64)
	Read(path []string, buff []byte, ofst int64, fh uint64) (n int)
}

// FuseFsWindows implements the interface between restic and fuse.
type FuseFsWindows struct {
	fuse.FileSystemBase
	lock     sync.Mutex
	ctx      context.Context
	repo     restic.Repository
	config   Config
	rootNode *FsNodeRoot
}

// NewFuseFsWindows initializes a new fuse filesystem for a repository.
func NewFuseFsWindows(ctx context.Context, repo restic.Repository, cfg Config) *FuseFsWindows {

	debug.Log("config: %v", cfg)

	snapshotManager := NewSnapshotManager(ctx, repo, cfg)
	snapshotManager.updateSnapshots()

	rootNode := NewFsNodeRoot(ctx, repo, cfg, *snapshotManager)

	fuseFsWindows := &FuseFsWindows{
		ctx:      ctx,
		repo:     repo,
		config:   cfg,
		rootNode: rootNode,
	}

	return fuseFsWindows
}

// Readdir lists all items in the specified path. Results are returned
// through the given callback function.
func (self *FuseFsWindows) Readdir(
	path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64,
) (errc int) {

	defer self.synchronize()()

	debug.Log("Readdir(%v)", path)

	splitPath := splitPath(path)
	self.rootNode.Readdir(splitPath, fill)

	return 0
}

// Getattr fetches the attributes of the specified file or directory.
func (self *FuseFsWindows) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {

	defer self.synchronize()()

	debug.Log("FuseFsWindows: Getattr(%v)", path)

	splitPath := splitPath(path)
	if self.rootNode.GetAttributes(splitPath, stat) {
		return 0
	}

	return -fuse.ENOENT
}

// Open opens the file for the given path.
func (self *FuseFsWindows) Open(path string, flags int) (errc int, fh uint64) {

	defer self.synchronize()()

	debug.Log("Open(%v, %v)", path, flags)

	splitPath := splitPath(path)
	return self.rootNode.Open(splitPath, flags)
}

// Read reads data to the given buffer from the specified file.
func (self *FuseFsWindows) Read(
	path string, buff []byte, ofst int64, fh uint64,
) (n int) {

	defer self.synchronize()()

	splitPath := splitPath(path)
	return self.rootNode.Read(splitPath, buff, ofst, fh)
}

// synchronize is a helper to synchronize access to the fuse filesystem.
func (self *FuseFsWindows) synchronize() func() {

	self.lock.Lock()

	return func() {
		self.lock.Unlock()
	}
}
