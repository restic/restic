// +build !openbsd
// +build !solaris
// +build !windows

package fuse

import (
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"golang.org/x/net/context"

	"bazil.org/fuse/fs"
)

// Config holds settings for the fuse mount.
type Config struct {
	OwnerIsRoot bool
	Host        string
	Tags        []restic.TagList
	Paths       []string
}

// Root is the root node of the fuse mount of a repository.
type Root struct {
	repo          restic.Repository
	cfg           Config
	inode         uint64
	snapshots     restic.Snapshots
	blobSizeCache *BlobSizeCache

	*MetaDir
}

// ensure that *Root implements these interfaces
var _ = fs.HandleReadDirAller(&Root{})
var _ = fs.NodeStringLookuper(&Root{})

const rootInode = 1

// NewRoot initializes a new root node from a repository.
func NewRoot(ctx context.Context, repo restic.Repository, cfg Config) (*Root, error) {
	debug.Log("NewRoot(), config %v", cfg)

	snapshots := restic.FindFilteredSnapshots(ctx, repo, cfg.Host, cfg.Tags, cfg.Paths)
	debug.Log("found %d matching snapshots", len(snapshots))

	root := &Root{
		repo:          repo,
		inode:         rootInode,
		cfg:           cfg,
		snapshots:     snapshots,
		blobSizeCache: NewBlobSizeCache(ctx, repo.Index()),
	}

	entries := map[string]fs.Node{
		"snapshots": NewSnapshotsDir(root, fs.GenerateDynamicInode(root.inode, "snapshots"), snapshots),
		"tags":      NewTagsDir(root, fs.GenerateDynamicInode(root.inode, "tags"), snapshots),
		"hosts":     NewHostsDir(root, fs.GenerateDynamicInode(root.inode, "hosts"), snapshots),
	}

	root.MetaDir = NewMetaDir(root, rootInode, entries)

	return root, nil
}

// NewTagsDir returns a new directory containing entries, which in turn contains
// snapshots with this tag set.
func NewTagsDir(root *Root, inode uint64, snapshots restic.Snapshots) fs.Node {
	tags := make(map[string]restic.Snapshots)
	for _, sn := range snapshots {
		for _, tag := range sn.Tags {
			tags[tag] = append(tags[tag], sn)
		}
	}

	debug.Log("create tags dir with %d tags, inode %d", len(tags), inode)

	entries := make(map[string]fs.Node)
	for name, snapshots := range tags {
		debug.Log("  tag %v has %v snapshots", name, len(snapshots))
		entries[name] = NewSnapshotsDir(root, fs.GenerateDynamicInode(inode, name), snapshots)
	}

	return NewMetaDir(root, inode, entries)
}

// NewHostsDir returns a new directory containing hostnames, which in
// turn contains snapshots of a single host each.
func NewHostsDir(root *Root, inode uint64, snapshots restic.Snapshots) fs.Node {
	hosts := make(map[string]restic.Snapshots)
	for _, sn := range snapshots {
		hosts[sn.Hostname] = append(hosts[sn.Hostname], sn)
	}

	debug.Log("create hosts dir with %d snapshots, inode %d", len(hosts), inode)

	entries := make(map[string]fs.Node)
	for name, snapshots := range hosts {
		debug.Log("  host %v has %v snapshots", name, len(snapshots))
		entries[name] = NewSnapshotsDir(root, fs.GenerateDynamicInode(inode, name), snapshots)
	}

	return NewMetaDir(root, inode, entries)
}

// Root is just there to satisfy fs.Root, it returns itself.
func (r *Root) Root() (fs.Node, error) {
	debug.Log("Root()")
	return r, nil
}
