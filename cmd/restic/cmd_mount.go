// +build !openbsd

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/crypto"
	"github.com/restic/restic/pack"
	"github.com/restic/restic/repository"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type CmdMount struct {
	global *GlobalOptions
	ready  chan struct{}
}

func init() {
	_, err := parser.AddCommand("mount",
		"mount a repository",
		"The mount command mounts a repository read-only to a given directory",
		&CmdMount{
			global: &globalOpts,
			ready:  make(chan struct{}, 1),
		})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdMount) Usage() string {
	return "MOUNTPOINT"
}

func (cmd CmdMount) Execute(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("wrong number of parameters, Usage: %s", cmd.Usage())
	}

	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	err = repo.LoadIndex()
	if err != nil {
		return err
	}

	mountpoint := args[0]
	if _, err := os.Stat(mountpoint); os.IsNotExist(err) {
		cmd.global.Verbosef("Mountpoint %s doesn't exist, creating it\n", mountpoint)
		err = os.Mkdir(mountpoint, os.ModeDir|0700)
		if err != nil {
			return err
		}
	}
	c, err := fuse.Mount(
		mountpoint,
		fuse.ReadOnly(),
		fuse.FSName("restic"),
	)
	if err != nil {
		return err
	}

	root := fs.Tree{}
	root.Add("snapshots", &snapshots{
		repo:           repo,
		knownSnapshots: make(map[string]snapshotWithId),
	})

	cmd.global.Printf("Now serving %s at %s\n", repo.Backend().Location(), mountpoint)
	cmd.global.Printf("Don't forget to umount after quitting!\n")

	cmd.ready <- struct{}{}

	err = fs.Serve(c, &root)
	if err != nil {
		return err
	}

	<-c.Ready
	return c.MountError
}

type snapshotWithId struct {
	*restic.Snapshot
	backend.ID
}

// These lines statically ensure that a *snapshots implement the given
// interfaces; a misplaced refactoring of the implementation that breaks
// the interface will be catched by the compiler
var _ = fs.HandleReadDirAller(&snapshots{})
var _ = fs.NodeStringLookuper(&snapshots{})

type snapshots struct {
	repo *repository.Repository

	// knownSnapshots maps snapshot timestamp to the snapshot
	sync.RWMutex
	knownSnapshots map[string]snapshotWithId
}

func (sn *snapshots) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Inode = 0
	attr.Mode = os.ModeDir | 0555
	return nil
}

func (sn *snapshots) updateCache(ctx context.Context) error {
	sn.Lock()
	defer sn.Unlock()

	for id := range sn.repo.List(backend.Snapshot, ctx.Done()) {
		snapshot, err := restic.LoadSnapshot(sn.repo, id)
		if err != nil {
			return err
		}
		sn.knownSnapshots[snapshot.Time.Format(time.RFC3339)] = snapshotWithId{snapshot, id}
	}
	return nil
}
func (sn *snapshots) get(name string) (snapshot snapshotWithId, ok bool) {
	sn.Lock()
	snapshot, ok = sn.knownSnapshots[name]
	sn.Unlock()
	return snapshot, ok
}

func (sn *snapshots) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	err := sn.updateCache(ctx)
	if err != nil {
		return nil, err
	}

	sn.RLock()
	defer sn.RUnlock()

	ret := make([]fuse.Dirent, 0)
	for _, snapshot := range sn.knownSnapshots {
		ret = append(ret, fuse.Dirent{
			Inode: binary.BigEndian.Uint64(snapshot.ID[:8]),
			Type:  fuse.DT_Dir,
			Name:  snapshot.Time.Format(time.RFC3339),
		})
	}

	return ret, nil
}

func (sn *snapshots) Lookup(ctx context.Context, name string) (fs.Node, error) {
	snapshot, ok := sn.get(name)

	if !ok {
		// We don't know about it, update the cache
		err := sn.updateCache(ctx)
		if err != nil {
			return nil, err
		}
		snapshot, ok = sn.get(name)
		if !ok {
			// We still don't know about it, this time it really doesn't exist
			return nil, fuse.ENOENT
		}
	}

	return newDirFromSnapshot(sn.repo, snapshot)
}

// Statically ensure that *dir implement those interface
var _ = fs.HandleReadDirAller(&dir{})
var _ = fs.NodeStringLookuper(&dir{})

type dir struct {
	repo     *repository.Repository
	children map[string]*restic.Node
	inode    uint64
}

func newDir(repo *repository.Repository, node *restic.Node) (*dir, error) {
	tree, err := restic.LoadTree(repo, node.Subtree)
	if err != nil {
		return nil, err
	}
	children := make(map[string]*restic.Node)
	for _, child := range tree.Nodes {
		children[child.Name] = child
	}

	return &dir{
		repo:     repo,
		children: children,
		inode:    node.Inode,
	}, nil
}

func newDirFromSnapshot(repo *repository.Repository, snapshot snapshotWithId) (*dir, error) {
	tree, err := restic.LoadTree(repo, snapshot.Tree)
	if err != nil {
		return nil, err
	}
	children := make(map[string]*restic.Node)
	for _, node := range tree.Nodes {
		children[node.Name] = node
	}

	return &dir{
		repo:     repo,
		children: children,
		inode:    binary.BigEndian.Uint64(snapshot.ID),
	}, nil
}

func (d *dir) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = d.inode
	a.Mode = os.ModeDir | 0555
	return nil
}

func (d *dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	ret := make([]fuse.Dirent, 0, len(d.children))

	for _, node := range d.children {
		var typ fuse.DirentType
		switch {
		case node.Mode.IsDir():
			typ = fuse.DT_Dir
		case node.Mode.IsRegular():
			typ = fuse.DT_File
		}

		ret = append(ret, fuse.Dirent{
			Inode: node.Inode,
			Type:  typ,
			Name:  node.Name,
		})
	}

	return ret, nil
}

func (d *dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	child, ok := d.children[name]
	if !ok {
		return nil, fuse.ENOENT
	}
	switch {
	case child.Mode.IsDir():
		return newDir(d.repo, child)
	case child.Mode.IsRegular():
		return newFile(d.repo, child)
	default:
		return nil, fuse.ENOENT
	}
}

// Statically ensure that *file implements the given interface
var _ = fs.HandleReader(&file{})

type file struct {
	repo *repository.Repository
	node *restic.Node

	sizes []uint32

	// cleartext contents
	clearContent [][]byte
}

func newFile(repo *repository.Repository, node *restic.Node) (*file, error) {
	sizes := make([]uint32, len(node.Content))
	for i, blobId := range node.Content {
		_, _, _, length, err := repo.Index().Lookup(blobId)
		if err != nil {
			return nil, err
		}
		sizes[i] = uint32(length) - crypto.Extension
	}

	return &file{
		repo:         repo,
		node:         node,
		sizes:        sizes,
		clearContent: make([][]byte, len(node.Content)),
	}, nil
}

func (f *file) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = f.node.Inode
	a.Mode = f.node.Mode
	a.Size = f.node.Size
	return nil
}

func (f *file) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	off := req.Offset
	content := make([]byte, req.Size)
	allContent := content

	for i := range f.node.Content {
		if off >= int64(f.sizes[i]) {
			off -= int64(f.sizes[i])
			continue
		}

		var subContent []byte
		if f.clearContent[i] != nil {
			subContent = f.clearContent[i]
		} else {
			var err error
			subContent, err = f.repo.LoadBlob(pack.Data, f.node.Content[i])
			if err != nil {
				return err
			}
			f.clearContent[i] = subContent
		}

		subContent = subContent[off:]
		off = 0

		var copied int
		if len(subContent) > len(content) {
			copied = copy(content[0:], subContent[:len(content)])
		} else {
			copied = copy(content[0:], subContent)
		}
		content = content[copied:]
		if len(content) == 0 {
			break
		}
	}
	resp.Data = allContent
	return nil
}
