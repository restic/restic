// +build !openbsd
// +build !solaris
// +build !windows

package fuse

import (
	"fmt"
	"os"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"

	"golang.org/x/net/context"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// SnapshotsDir is a fuse directory which contains snapshots named by timestamp.
type SnapshotsDir struct {
	inode   uint64
	root    *Root
	names   map[string]*restic.Snapshot
	latest  string
	tag     string
	host    string
	snCount int

	template string
}

// SnapshotsIDSDir is a fuse directory which contains snapshots named by ids.
type SnapshotsIDSDir struct {
	inode   uint64
	root    *Root
	names   map[string]*restic.Snapshot
	snCount int
}

// HostsDir is a fuse directory which contains hosts.
type HostsDir struct {
	inode   uint64
	root    *Root
	hosts   map[string]bool
	snCount int
}

// TagsDir is a fuse directory which contains tags.
type TagsDir struct {
	inode   uint64
	root    *Root
	tags    map[string]bool
	snCount int
}

// SnapshotLink
type snapshotLink struct {
	root     *Root
	inode    uint64
	target   string
	snapshot *restic.Snapshot
}

// ensure that *SnapshotsDir implements these interfaces
var _ = fs.HandleReadDirAller(&SnapshotsDir{})
var _ = fs.NodeStringLookuper(&SnapshotsDir{})
var _ = fs.HandleReadDirAller(&SnapshotsIDSDir{})
var _ = fs.NodeStringLookuper(&SnapshotsIDSDir{})
var _ = fs.HandleReadDirAller(&TagsDir{})
var _ = fs.NodeStringLookuper(&TagsDir{})
var _ = fs.HandleReadDirAller(&HostsDir{})
var _ = fs.NodeStringLookuper(&HostsDir{})
var _ = fs.NodeReadlinker(&snapshotLink{})

// read tag names from the current repository-state.
func updateTagNames(d *TagsDir) {
	if d.snCount != d.root.snCount {
		d.snCount = d.root.snCount
		d.tags = make(map[string]bool, len(d.root.snapshots))
		for _, snapshot := range d.root.snapshots {
			for _, tag := range snapshot.Tags {
				if tag != "" {
					d.tags[tag] = true
				}
			}
		}
	}
}

// read host names from the current repository-state.
func updateHostsNames(d *HostsDir) {
	if d.snCount != d.root.snCount {
		d.snCount = d.root.snCount
		d.hosts = make(map[string]bool, len(d.root.snapshots))
		for _, snapshot := range d.root.snapshots {
			d.hosts[snapshot.Hostname] = true
		}
	}
}

// read snapshot id names from the current repository-state.
func updateSnapshotIDSNames(d *SnapshotsIDSDir) {
	if d.snCount != d.root.snCount {
		d.snCount = d.root.snCount
		for _, sn := range d.root.snapshots {
			name := sn.ID().Str()
			d.names[name] = sn
		}
	}
}

// NewSnapshotsDir returns a new directory containing snapshots.
func NewSnapshotsDir(root *Root, inode uint64, tag string, host string) *SnapshotsDir {
	debug.Log("create snapshots dir, inode %d", inode)
	d := &SnapshotsDir{
		root:     root,
		inode:    inode,
		names:    make(map[string]*restic.Snapshot),
		latest:   "",
		tag:      tag,
		host:     host,
		template: root.cfg.SnapshotTemplate,
	}

	return d
}

// NewSnapshotsIDSDir returns a new directory containing snapshots named by ids.
func NewSnapshotsIDSDir(root *Root, inode uint64) *SnapshotsIDSDir {
	debug.Log("create snapshots ids dir, inode %d", inode)
	d := &SnapshotsIDSDir{
		root:  root,
		inode: inode,
		names: make(map[string]*restic.Snapshot),
	}

	return d
}

// NewHostsDir returns a new directory containing host names
func NewHostsDir(root *Root, inode uint64) *HostsDir {
	debug.Log("create hosts dir, inode %d", inode)
	d := &HostsDir{
		root:  root,
		inode: inode,
		hosts: make(map[string]bool),
	}

	return d
}

// NewTagsDir returns a new directory containing tag names
func NewTagsDir(root *Root, inode uint64) *TagsDir {
	debug.Log("create tags dir, inode %d", inode)
	d := &TagsDir{
		root:  root,
		inode: inode,
		tags:  make(map[string]bool),
	}

	return d
}

// Attr returns the attributes for the root node.
func (d *SnapshotsDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Inode = d.inode
	attr.Mode = os.ModeDir | 0555

	if !d.root.cfg.OwnerIsRoot {
		attr.Uid = uint32(os.Getuid())
		attr.Gid = uint32(os.Getgid())
	}
	debug.Log("attr: %v", attr)
	return nil
}

// Attr returns the attributes for the SnapshotsDir.
func (d *SnapshotsIDSDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Inode = d.inode
	attr.Mode = os.ModeDir | 0555

	if !d.root.cfg.OwnerIsRoot {
		attr.Uid = uint32(os.Getuid())
		attr.Gid = uint32(os.Getgid())
	}
	debug.Log("attr: %v", attr)
	return nil
}

// Attr returns the attributes for the HostsDir.
func (d *HostsDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Inode = d.inode
	attr.Mode = os.ModeDir | 0555

	if !d.root.cfg.OwnerIsRoot {
		attr.Uid = uint32(os.Getuid())
		attr.Gid = uint32(os.Getgid())
	}
	debug.Log("attr: %v", attr)
	return nil
}

// Attr returns the attributes for the TagsDir.
func (d *TagsDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Inode = d.inode
	attr.Mode = os.ModeDir | 0555

	if !d.root.cfg.OwnerIsRoot {
		attr.Uid = uint32(os.Getuid())
		attr.Gid = uint32(os.Getgid())
	}
	debug.Log("attr: %v", attr)
	return nil
}

// search element in string list.
func isElem(e string, list []string) bool {
	for _, x := range list {
		if e == x {
			return true
		}
	}
	return false
}

const minSnapshotsReloadTime = 60 * time.Second

// update snapshots if repository has changed
func updateSnapshots(ctx context.Context, root *Root) error {
	if time.Since(root.lastCheck) < minSnapshotsReloadTime {
		return nil
	}

	snapshots, err := restic.FindFilteredSnapshots(ctx, root.repo, root.cfg.Host, root.cfg.Tags, root.cfg.Paths)
	if err != nil {
		return err
	}

	if root.snCount != len(snapshots) {
		root.snCount = len(snapshots)
		root.repo.LoadIndex(ctx)
		root.snapshots = snapshots
	}
	root.lastCheck = time.Now()

	return nil
}

// read snapshot timestamps from the current repository-state.
func updateSnapshotNames(d *SnapshotsDir, template string) {
	if d.snCount != d.root.snCount {
		d.snCount = d.root.snCount
		var latestTime time.Time
		d.latest = ""
		d.names = make(map[string]*restic.Snapshot, len(d.root.snapshots))
		for _, sn := range d.root.snapshots {
			if d.tag == "" || isElem(d.tag, sn.Tags) {
				if d.host == "" || d.host == sn.Hostname {
					name := sn.Time.Format(template)
					if d.latest == "" || !sn.Time.Before(latestTime) {
						latestTime = sn.Time
						d.latest = name
					}
					for i := 1; ; i++ {
						if _, ok := d.names[name]; !ok {
							break
						}

						name = fmt.Sprintf("%s-%d", sn.Time.Format(template), i)
					}

					d.names[name] = sn
				}
			}
		}
	}
}

// ReadDirAll returns all entries of the SnapshotsDir.
func (d *SnapshotsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	debug.Log("ReadDirAll()")

	// update snapshots
	updateSnapshots(ctx, d.root)

	// update snapshot names
	updateSnapshotNames(d, d.root.cfg.SnapshotTemplate)

	items := []fuse.Dirent{
		{
			Inode: d.inode,
			Name:  ".",
			Type:  fuse.DT_Dir,
		},
		{
			Inode: d.root.inode,
			Name:  "..",
			Type:  fuse.DT_Dir,
		},
	}

	for name := range d.names {
		items = append(items, fuse.Dirent{
			Inode: fs.GenerateDynamicInode(d.inode, name),
			Name:  name,
			Type:  fuse.DT_Dir,
		})
	}

	// Latest
	if d.latest != "" {
		items = append(items, fuse.Dirent{
			Inode: fs.GenerateDynamicInode(d.inode, "latest"),
			Name:  "latest",
			Type:  fuse.DT_Link,
		})
	}
	return items, nil
}

// ReadDirAll returns all entries of the SnapshotsIDSDir.
func (d *SnapshotsIDSDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	debug.Log("ReadDirAll()")

	// update snapshots
	updateSnapshots(ctx, d.root)

	// update snapshot ids
	updateSnapshotIDSNames(d)

	items := []fuse.Dirent{
		{
			Inode: d.inode,
			Name:  ".",
			Type:  fuse.DT_Dir,
		},
		{
			Inode: d.root.inode,
			Name:  "..",
			Type:  fuse.DT_Dir,
		},
	}

	for name := range d.names {
		items = append(items, fuse.Dirent{
			Inode: fs.GenerateDynamicInode(d.inode, name),
			Name:  name,
			Type:  fuse.DT_Dir,
		})
	}

	return items, nil
}

// ReadDirAll returns all entries of the HostsDir.
func (d *HostsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	debug.Log("ReadDirAll()")

	// update snapshots
	updateSnapshots(ctx, d.root)

	// update host names
	updateHostsNames(d)

	items := []fuse.Dirent{
		{
			Inode: d.inode,
			Name:  ".",
			Type:  fuse.DT_Dir,
		},
		{
			Inode: d.root.inode,
			Name:  "..",
			Type:  fuse.DT_Dir,
		},
	}

	for host := range d.hosts {
		items = append(items, fuse.Dirent{
			Inode: fs.GenerateDynamicInode(d.inode, host),
			Name:  host,
			Type:  fuse.DT_Dir,
		})
	}

	return items, nil
}

// ReadDirAll returns all entries of the TagsDir.
func (d *TagsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	debug.Log("ReadDirAll()")

	// update snapshots
	updateSnapshots(ctx, d.root)

	// update tag names
	updateTagNames(d)

	items := []fuse.Dirent{
		{
			Inode: d.inode,
			Name:  ".",
			Type:  fuse.DT_Dir,
		},
		{
			Inode: d.root.inode,
			Name:  "..",
			Type:  fuse.DT_Dir,
		},
	}

	for tag := range d.tags {
		items = append(items, fuse.Dirent{
			Inode: fs.GenerateDynamicInode(d.inode, tag),
			Name:  tag,
			Type:  fuse.DT_Dir,
		})
	}

	return items, nil
}

// newSnapshotLink
func newSnapshotLink(ctx context.Context, root *Root, inode uint64, target string, snapshot *restic.Snapshot) (*snapshotLink, error) {
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

	if !l.root.cfg.OwnerIsRoot {
		a.Uid = uint32(os.Getuid())
		a.Gid = uint32(os.Getgid())
	}
	a.Atime = l.snapshot.Time
	a.Ctime = l.snapshot.Time
	a.Mtime = l.snapshot.Time

	a.Nlink = 1

	return nil
}

// Lookup returns a specific entry from the SnapshotsDir.
func (d *SnapshotsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("Lookup(%s)", name)

	sn, ok := d.names[name]
	if !ok {
		// could not find entry. Updating repository-state
		updateSnapshots(ctx, d.root)

		// update snapshot names
		updateSnapshotNames(d, d.root.cfg.SnapshotTemplate)

		sn, ok := d.names[name]
		if ok {
			return newDirFromSnapshot(ctx, d.root, fs.GenerateDynamicInode(d.inode, name), sn)
		}

		if name == "latest" && d.latest != "" {
			sn, ok := d.names[d.latest]

			// internal error
			if !ok {
				return nil, fuse.ENOENT
			}

			return newSnapshotLink(ctx, d.root, fs.GenerateDynamicInode(d.inode, name), d.latest, sn)
		}
		return nil, fuse.ENOENT
	}

	return newDirFromSnapshot(ctx, d.root, fs.GenerateDynamicInode(d.inode, name), sn)
}

// Lookup returns a specific entry from the SnapshotsIDSDir.
func (d *SnapshotsIDSDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("Lookup(%s)", name)

	sn, ok := d.names[name]
	if !ok {
		// could not find entry. Updating repository-state
		updateSnapshots(ctx, d.root)

		// update snapshot ids
		updateSnapshotIDSNames(d)

		sn, ok := d.names[name]
		if ok {
			return newDirFromSnapshot(ctx, d.root, fs.GenerateDynamicInode(d.inode, name), sn)
		}

		return nil, fuse.ENOENT
	}

	return newDirFromSnapshot(ctx, d.root, fs.GenerateDynamicInode(d.inode, name), sn)
}

// Lookup returns a specific entry from the HostsDir.
func (d *HostsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("Lookup(%s)", name)

	_, ok := d.hosts[name]
	if !ok {
		// could not find entry. Updating repository-state
		updateSnapshots(ctx, d.root)

		// update host names
		updateHostsNames(d)

		_, ok := d.hosts[name]
		if ok {
			return NewSnapshotsDir(d.root, fs.GenerateDynamicInode(d.root.inode, name), "", name), nil
		}

		return nil, fuse.ENOENT
	}

	return NewSnapshotsDir(d.root, fs.GenerateDynamicInode(d.root.inode, name), "", name), nil
}

// Lookup returns a specific entry from the TagsDir.
func (d *TagsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("Lookup(%s)", name)

	_, ok := d.tags[name]
	if !ok {
		// could not find entry. Updating repository-state
		updateSnapshots(ctx, d.root)

		// update tag names
		updateTagNames(d)

		_, ok := d.tags[name]
		if ok {
			return NewSnapshotsDir(d.root, fs.GenerateDynamicInode(d.root.inode, name), name, ""), nil
		}

		return nil, fuse.ENOENT
	}

	return NewSnapshotsDir(d.root, fs.GenerateDynamicInode(d.root.inode, name), name, ""), nil
}
