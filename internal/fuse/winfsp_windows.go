package fuse

import (
	"context"
	"io"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/winfsp/go-winfsp/gofs"

	"github.com/restic/restic/internal/bloblru"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

// Number of directory trees kept in memory.
const treeCacheSize = 512

// WinFS serves a repository read-only as a gofs.FileSystem, which go-winfsp
// exposes to Windows through WinFSP. In contrast to FUSE, WinFSP addresses
// files by path, thus every path is resolved through the snapshot directory
// structure and the trees of the selected snapshot.
type WinFS struct {
	ctx       context.Context
	repo      restic.Repository
	blobCache *bloblru.Cache
	dirStruct *SnapshotsDirStructure
	trees     *lru.Cache[restic.ID, map[string]*data.Node]
	mountTime time.Time
}

// ensure that *WinFS implements these interfaces
var _ gofs.FileSystem = (*WinFS)(nil)

// NewWinFS initializes a new WinFS from a repository. All repository accesses
// made while serving the file system are bound to ctx.
func NewWinFS(ctx context.Context, repo restic.Repository, cfg Config) *WinFS {
	debug.Log("NewWinFS(), config %v", cfg)

	if len(cfg.PathTemplates) == 0 {
		cfg.PathTemplates = defaultPathTemplates
	}

	trees, err := lru.New[restic.ID, map[string]*data.Node](treeCacheSize)
	if err != nil {
		// only fails for a non-positive size
		panic(err)
	}

	return &WinFS{
		ctx:       ctx,
		repo:      repo,
		blobCache: bloblru.New(blobCacheSize),
		dirStruct: NewSnapshotsDirStructure(repo, cfg.Filter, cfg.PathTemplates, cfg.TimeTemplate),
		trees:     trees,
		mountTime: time.Now(),
	}
}

// splitPath splits a path as passed by gofs, e.g. `\ids\1234abcd\dir`, into
// its components. The root directory has no components.
func splitPath(name string) []string {
	return strings.FieldsFunc(name, func(r rune) bool { return r == '\\' || r == '/' })
}

// metaNode returns the node for a directory generated from the snapshot
// directory structure.
func metaNode(name string, modTime time.Time) *data.Node {
	return &data.Node{
		Name:       name,
		Type:       data.NodeTypeDir,
		Mode:       os.ModeDir | 0555,
		AccessTime: modTime,
		ModTime:    modTime,
		ChangeTime: modTime,
	}
}

// snapshotNode returns the node for the root directory of a snapshot.
func snapshotNode(name string, sn *data.Snapshot) *data.Node {
	node := metaNode(name, sn.Time)
	node.Subtree = sn.Tree
	return node
}

// isVisible reports whether node can be represented on a WinFSP mount. Symlinks
// and special files have no representation without reparse point support and
// are therefore hidden.
func isVisible(node *data.Node) bool {
	return node.Type == data.NodeTypeDir || node.Type == data.NodeTypeFile
}

func (fs *WinFS) metaDir(prefix string) (*MetaDirData, error) {
	meta, _, err := fs.dirStruct.UpdatePrefix(fs.ctx, prefix)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, os.ErrNotExist
	}
	return meta, nil
}

func (fs *WinFS) treeItems(id restic.ID) (map[string]*data.Node, error) {
	if items, ok := fs.trees.Get(id); ok {
		return items, nil
	}
	items, err := loadTreeItems(fs.ctx, fs.repo, id)
	if err != nil {
		return nil, err
	}
	fs.trees.Add(id, items)
	return items, nil
}

// resolve looks up name and returns the node describing it. For directories
// generated from the snapshot directory structure, the MetaDirData listing
// their entries is returned as well.
func (fs *WinFS) resolve(name string) (*data.Node, *MetaDirData, error) {
	comps := splitPath(name)
	for i := range comps {
		comps[i] = unescapeWindowsName(comps[i])
	}
	prefix := ""
	node := metaNode(`\`, fs.mountTime)
	for i, comp := range comps {
		meta, err := fs.metaDir(prefix)
		if err != nil {
			return nil, nil, err
		}
		entry := meta.names[comp]
		if entry == nil {
			return nil, nil, os.ErrNotExist
		}
		if entry.snapshot != nil {
			// "latest" links are resolved to the snapshot they point to
			return fs.resolveInTree(snapshotNode(comp, entry.snapshot), comps[i+1:])
		}
		prefix += "/" + comp
		node = metaNode(comp, fs.mountTime)
	}

	meta, err := fs.metaDir(prefix)
	if err != nil {
		return nil, nil, err
	}
	return node, meta, nil
}

func (fs *WinFS) resolveInTree(node *data.Node, comps []string) (*data.Node, *MetaDirData, error) {
	for _, comp := range comps {
		if node.Type != data.NodeTypeDir || node.Subtree == nil {
			return nil, nil, syscall.ENOTDIR
		}
		items, err := fs.treeItems(*node.Subtree)
		if err != nil {
			return nil, nil, err
		}
		next, ok := items[comp]
		if !ok || !isVisible(next) {
			return nil, nil, os.ErrNotExist
		}
		node = next
	}
	return node, nil, nil
}

func (fs *WinFS) Stat(name string) (os.FileInfo, error) {
	debug.Log("Stat(%v)", name)
	node, _, err := fs.resolve(name)
	if err != nil {
		return nil, err
	}
	return winFileInfo{node}, nil
}

const writeFlags = os.O_WRONLY | os.O_RDWR | os.O_APPEND | os.O_CREATE | os.O_TRUNC

func (fs *WinFS) OpenFile(name string, flag int, _ os.FileMode) (gofs.File, error) {
	debug.Log("OpenFile(%v, %v)", name, flag)
	if flag&writeFlags != 0 {
		return nil, os.ErrPermission
	}
	node, meta, err := fs.resolve(name)
	if err != nil {
		return nil, err
	}
	if node.Type == data.NodeTypeDir {
		return fs.openDir(node, meta)
	}
	return fs.openFile(node)
}

func (fs *WinFS) Mkdir(string, os.FileMode) error { return os.ErrPermission }
func (fs *WinFS) Rename(string, string) error     { return os.ErrPermission }
func (fs *WinFS) Remove(string) error             { return os.ErrPermission }

// winFileInfo is the os.FileInfo of a node. All entries are read-only. Names
// are escaped such that they are valid on Windows, see escapeWindowsName.
type winFileInfo struct {
	node *data.Node
}

func (i winFileInfo) Name() string       { return escapeWindowsName(cleanupNodeName(i.node.Name)) }
func (i winFileInfo) Size() int64        { return int64(i.node.Size) }
func (i winFileInfo) ModTime() time.Time { return i.node.ModTime }
func (i winFileInfo) IsDir() bool        { return i.node.Type == data.NodeTypeDir }
func (i winFileInfo) Sys() any           { return nil }

func (i winFileInfo) Mode() os.FileMode {
	if i.IsDir() {
		return os.ModeDir | 0555
	}
	return 0444
}

// readOnlyFile provides the gofs.File methods that are identical for files and
// directories of a read-only file system.
type readOnlyFile struct{}

func (readOnlyFile) Close() error                       { return nil }
func (readOnlyFile) Sync() error                        { return nil }
func (readOnlyFile) Write([]byte) (int, error)          { return 0, os.ErrPermission }
func (readOnlyFile) WriteAt([]byte, int64) (int, error) { return 0, os.ErrPermission }
func (readOnlyFile) Truncate(int64) error               { return os.ErrPermission }

// winDir is an open directory. Its entries are captured while opening.
type winDir struct {
	readOnlyFile
	node    *data.Node
	entries []os.FileInfo
	pos     int
}

var _ gofs.File = (*winDir)(nil)

func (fs *WinFS) openDir(node *data.Node, meta *MetaDirData) (*winDir, error) {
	debug.Log("open dir %v (%v)", node.Name, node.Subtree)

	var entries []os.FileInfo
	switch {
	case meta != nil:
		for name, entry := range meta.names {
			modTime := fs.mountTime
			if entry.snapshot != nil {
				modTime = entry.snapshot.Time
			}
			entries = append(entries, winFileInfo{metaNode(name, modTime)})
		}
	case node.Subtree != nil:
		items, err := fs.treeItems(*node.Subtree)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if isVisible(item) {
				entries = append(entries, winFileInfo{item})
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	return &winDir{node: node, entries: entries}, nil
}

func (d *winDir) Readdir(count int) ([]os.FileInfo, error) {
	if d.pos >= len(d.entries) {
		if count > 0 {
			return nil, io.EOF
		}
		return nil, nil
	}
	end := len(d.entries)
	if count > 0 && d.pos+count < end {
		end = d.pos + count
	}
	entries := d.entries[d.pos:end]
	d.pos = end
	return entries, nil
}

func (d *winDir) Stat() (os.FileInfo, error)        { return winFileInfo{d.node}, nil }
func (d *winDir) Read([]byte) (int, error)          { return 0, syscall.EISDIR }
func (d *winDir) ReadAt([]byte, int64) (int, error) { return 0, syscall.EISDIR }
func (d *winDir) Seek(int64, int) (int64, error)    { return 0, syscall.EISDIR }
