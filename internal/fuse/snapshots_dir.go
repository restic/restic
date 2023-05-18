//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fuse

import (
	"context"
	"os"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"github.com/anacrolix/fuse"
	"github.com/anacrolix/fuse/fs"
)

// SnapshotsDir is a actual fuse directory generated from SnapshotsDirStructure
// It uses the saved prefix to select the corresponding MetaDirData.
type SnapshotsDir struct {
	root        *Root
	inode       uint64
	parentInode uint64
	dirStruct   *SnapshotsDirStructure
	prefix      string
}

// ensure that *SnapshotsDir implements these interfaces
var _ = fs.HandleReadDirAller(&SnapshotsDir{})
var _ = fs.NodeStringLookuper(&SnapshotsDir{})

// NewSnapshotsDir returns a new directory structure containing snapshots and "latest" links
func NewSnapshotsDir(root *Root, inode, parentInode uint64, dirStruct *SnapshotsDirStructure, prefix string) *SnapshotsDir {
	debug.Log("create snapshots dir, inode %d", inode)
	return &SnapshotsDir{
		root:        root,
		inode:       inode,
		parentInode: parentInode,
		dirStruct:   dirStruct,
		prefix:      prefix,
	}
}

// Attr returns the attributes for any dir in the snapshots directory structure
func (d *SnapshotsDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Inode = d.inode
	attr.Mode = os.ModeDir | 0555
	attr.Uid = d.root.uid
	attr.Gid = d.root.gid

	debug.Log("attr: %v", attr)
	return nil
}

// ReadDirAll returns all entries of the SnapshotsDir.
func (d *SnapshotsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	debug.Log("ReadDirAll()")

	// update snapshots
	meta, err := d.dirStruct.UpdatePrefix(ctx, d.prefix)
	if err != nil {
		return nil, unwrapCtxCanceled(err)
	} else if meta == nil {
		return nil, fuse.ENOENT
	}

	items := []fuse.Dirent{
		{
			Inode: d.inode,
			Name:  ".",
			Type:  fuse.DT_Dir,
		},
		{
			Inode: d.parentInode,
			Name:  "..",
			Type:  fuse.DT_Dir,
		},
	}

	for name, entry := range meta.names {
		d := fuse.Dirent{
			Inode: inodeFromName(d.inode, name),
			Name:  name,
			Type:  fuse.DT_Dir,
		}
		if entry.linkTarget != "" {
			d.Type = fuse.DT_Link
		}
		items = append(items, d)
	}

	return items, nil
}

// Lookup returns a specific entry from the SnapshotsDir.
func (d *SnapshotsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("Lookup(%s)", name)

	meta, err := d.dirStruct.UpdatePrefix(ctx, d.prefix)
	if err != nil {
		return nil, unwrapCtxCanceled(err)
	} else if meta == nil {
		return nil, fuse.ENOENT
	}

	entry := meta.names[name]
	if entry != nil {
		inode := inodeFromName(d.inode, name)
		if entry.linkTarget != "" {
			return newSnapshotLink(d.root, inode, entry.linkTarget, entry.snapshot)
		} else if entry.snapshot != nil {
			return newDirFromSnapshot(d.root, inode, entry.snapshot)
		} else {
			return NewSnapshotsDir(d.root, inode, d.inode, d.dirStruct, d.prefix+"/"+name), nil
		}
	}

	return nil, fuse.ENOENT
}

// SnapshotLink
type snapshotLink struct {
	root     *Root
	inode    uint64
	target   string
	snapshot *restic.Snapshot
}

var _ = fs.NodeReadlinker(&snapshotLink{})

// newSnapshotLink
func newSnapshotLink(root *Root, inode uint64, target string, snapshot *restic.Snapshot) (*snapshotLink, error) {
	return &snapshotLink{root: root, inode: inode, target: target, snapshot: snapshot}, nil
}

// Readlink
func (l *snapshotLink) Readlink(ctx context.Context, req *fuse.ReadlinkRequest) (string, error) {
	return l.target, nil
}

// Attr
func (l *snapshotLink) Attr(ctx context.Context, a *fuse.Attr) error {
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
