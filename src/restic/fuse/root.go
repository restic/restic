// +build !openbsd
// +build !windows

package fuse

import (
	"os"
	"restic"
	"restic/debug"

	"golang.org/x/net/context"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// Config holds settings for the fuse mount.
type Config struct {
	OwnerIsRoot bool
	Host        string
	Tags        []string
	Paths       []string
}

// Root is the root node of the fuse mount of a repository.
type Root struct {
	repo          restic.Repository
	cfg           Config
	inode         uint64
	snapshots     restic.Snapshots
	dirSnapshots  *SnapshotsDir
	blobSizeCache *BlobSizeCache
}

// ensure that *Root implements these interfaces
var _ = fs.HandleReadDirAller(&Root{})
var _ = fs.NodeStringLookuper(&Root{})

// NewRoot initializes a new root node from a repository.
func NewRoot(ctx context.Context, repo restic.Repository, cfg Config) (*Root, error) {
	debug.Log("NewRoot(), config %v", cfg)

	snapshots := restic.FindFilteredSnapshots(ctx, repo, cfg.Host, cfg.Tags, cfg.Paths)
	debug.Log("found %d matching snapshots", len(snapshots))

	root := &Root{
		repo:      repo,
		cfg:       cfg,
		inode:     1,
		snapshots: snapshots,
	}

	root.dirSnapshots = NewDirSnapshots(root, fs.GenerateDynamicInode(root.inode, "snapshots"), snapshots)
	root.blobSizeCache = NewBlobSizeCache(ctx, repo.Index())

	return root, nil
}

// Root is just there to satisfy fs.Root, it returns itself.
func (r *Root) Root() (fs.Node, error) {
	debug.Log("Root()")
	return r, nil
}

// Attr returns the attributes for the root node.
func (r *Root) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Inode = r.inode
	attr.Mode = os.ModeDir | 0555

	if !r.cfg.OwnerIsRoot {
		attr.Uid = uint32(os.Getuid())
		attr.Gid = uint32(os.Getgid())
	}
	debug.Log("attr: %v", attr)
	return nil
}

// ReadDirAll returns all entries of the root node.
func (r *Root) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	debug.Log("ReadDirAll()")
	items := []fuse.Dirent{
		{
			Inode: r.inode,
			Name:  ".",
			Type:  fuse.DT_Dir,
		},
		{
			Inode: r.inode,
			Name:  "..",
			Type:  fuse.DT_Dir,
		},
		{
			Inode: fs.GenerateDynamicInode(r.inode, "snapshots"),
			Name:  "snapshots",
			Type:  fuse.DT_Dir,
		},
		// {
		// 	Inode: fs.GenerateDynamicInode(0, "tags"),
		// 	Name:  "tags",
		// 	Type:  fuse.DT_Dir,
		// },
		// {
		// 	Inode: fs.GenerateDynamicInode(0, "hosts"),
		// 	Name:  "hosts",
		// 	Type:  fuse.DT_Dir,
		// },
	}

	return items, nil
}

// Lookup returns a specific entry from the root node.
func (r *Root) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("Lookup(%s)", name)
	switch name {
	case "snapshots":
		return r.dirSnapshots, nil
	}

	return nil, fuse.ENOENT
}
