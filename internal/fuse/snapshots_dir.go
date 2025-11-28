//go:build darwin || freebsd || linux || windows
// +build darwin freebsd linux windows

package fuse

import (
	"context"
	"os"
	"syscall"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
)

// SnapshotsDir is a actual fuse directory generated from SnapshotsDirStructure
// It uses the saved prefix to select the corresponding MetaDirData.
type SnapshotsDir struct {
	root        *Root
	forget      forgetFn
	inode       uint64
	parentInode uint64
	dirStruct   *SnapshotsDirStructure
	prefix      string
	cache       treeCache
}

// ensure that *SnapshotsDir implements these interfaces
var _ = HandleReadDirAller(&SnapshotsDir{})
var _ = NodeForgetter(&SnapshotsDir{})
var _ = NodeStringLookuper(&SnapshotsDir{})

// NewSnapshotsDir returns a new directory structure containing snapshots and "latest" links
func NewSnapshotsDir(root *Root, forget forgetFn, inode, parentInode uint64, dirStruct *SnapshotsDirStructure, prefix string) *SnapshotsDir {
	debug.Log("create snapshots dir, inode %d", inode)
	return &SnapshotsDir{
		root:        root,
		forget:      forget,
		inode:       inode,
		parentInode: parentInode,
		dirStruct:   dirStruct,
		prefix:      prefix,
		cache:       *newTreeCache(),
	}
}

// Attr returns the attributes for any dir in the snapshots directory structure
func (d *SnapshotsDir) Attr(_ context.Context, attr *Attr) error {
	attr.Inode = d.inode
	attr.Mode = os.ModeDir | 0555
	attr.Uid = d.root.uid
	attr.Gid = d.root.gid

	debug.Log("attr: %v", attr)
	return nil
}

// ReadDirAll returns all entries of the SnapshotsDir.
func (d *SnapshotsDir) ReadDirAll(ctx context.Context) ([]Dirent, error) {
	debug.Log("ReadDirAll()")

	// update snapshots
	meta, err := d.dirStruct.UpdatePrefix(ctx, d.prefix)
	if err != nil {
		return nil, unwrapCtxCanceled(err)
	} else if meta == nil {
		return nil, syscall.ENOENT
	}

	items := []Dirent{
		{
			Inode: d.inode,
			Name:  ".",
			Type:  DT_Dir,
		},
		{
			Inode: d.parentInode,
			Name:  "..",
			Type:  DT_Dir,
		},
	}

	for name, entry := range meta.names {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		d := Dirent{
			Inode: inodeFromName(d.inode, name),
			Name:  name,
			Type:  DT_Dir,
		}
		if entry.linkTarget != "" {
			d.Type = DT_Link
		}
		items = append(items, d)
	}

	return items, nil
}

// Lookup returns a specific entry from the SnapshotsDir.
func (d *SnapshotsDir) Lookup(ctx context.Context, name string) (Node, error) {
	debug.Log("Lookup(%s)", name)

	meta, err := d.dirStruct.UpdatePrefix(ctx, d.prefix)
	if err != nil {
		return nil, unwrapCtxCanceled(err)
	} else if meta == nil {
		return nil, syscall.ENOENT
	}

	return d.cache.lookupOrCreate(name, func(forget forgetFn) (Node, error) {
		entry := meta.names[name]
		if entry == nil {
			return nil, syscall.ENOENT
		}

		inode := inodeFromName(d.inode, name)
		if entry.linkTarget != "" {
			return newSnapshotLink(d.root, forget, inode, entry.linkTarget, entry.snapshot)
		} else if entry.snapshot != nil {
			return newDirFromSnapshot(d.root, forget, inode, entry.snapshot)
		}
		return NewSnapshotsDir(d.root, forget, inode, d.inode, d.dirStruct, d.prefix+"/"+name), nil
	})
}

func (d *SnapshotsDir) Forget() {
	d.forget()
}

// SnapshotLink
type snapshotLink struct {
	root     *Root
	forget   forgetFn
	inode    uint64
	target   string
	snapshot *data.Snapshot
}

var _ = NodeForgetter(&snapshotLink{})
var _ = NodeReadlinker(&snapshotLink{})

// newSnapshotLink
func newSnapshotLink(root *Root, forget forgetFn, inode uint64, target string, snapshot *data.Snapshot) (*snapshotLink, error) {
	return &snapshotLink{root: root, forget: forget, inode: inode, target: target, snapshot: snapshot}, nil
}

// Readlink
func (l *snapshotLink) Readlink(_ context.Context, _ *ReadlinkRequest) (string, error) {
	return l.target, nil
}

// Attr
func (l *snapshotLink) Attr(_ context.Context, a *Attr) error {
	a.Inode = l.inode
	a.Mode = os.ModeSymlink | 0777
	a.Size = uint64(len(l.target))
	a.Blocks = (a.Size + blockSize - 1) / blockSize
	a.Uid = l.root.uid
	a.Gid = l.root.gid
	a.Atime = l.snapshot.Time
	a.Ctime = l.snapshot.Time
	a.Mtime = l.snapshot.Time

	a.Nlink = 1

	return nil
}

func (l *snapshotLink) Forget() {
	l.forget()
}
