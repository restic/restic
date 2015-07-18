package main

import (
	"encoding/binary"
	"fmt"
	"os"
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
}

func init() {
	_, err := parser.AddCommand("mount",
		"mount a repository",
		"The mount command mounts a repository read-only to a given directory",
		&CmdMount{global: &globalOpts})
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
	if _, err := os.Stat(mountpoint); err != nil {
		if os.IsNotExist(err) {
			cmd.global.Verbosef("Mountpoint [%s] doesn't exist, creating it\n", mountpoint)
			err = os.Mkdir(mountpoint, os.ModeDir|0755)
			if err != nil {
				return err
			}
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
	root.Add("snapshots", &snapshots{repo})

	cmd.global.Printf("Now serving %s under %s\n", repo.Backend().Location(), mountpoint)
	cmd.global.Printf("Don't forget to umount after quitting !\n")

	err = fs.Serve(c, &root)
	if err != nil {
		return err
	}

	<-c.Ready
	return c.MountError
}

var _ = fs.HandleReadDirAller(&snapshots{})
var _ = fs.NodeStringLookuper(&snapshots{})

type snapshots struct {
	repo *repository.Repository
}

func (sn *snapshots) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = 0
	a.Mode = os.ModeDir | 0555
	return nil
}

func (sn *snapshots) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	ret := make([]fuse.Dirent, 0)
	for id := range sn.repo.List(backend.Snapshot, ctx.Done()) {
		snapshot, err := restic.LoadSnapshot(sn.repo, id)
		if err != nil {
			return nil, err
		}
		ret = append(ret, fuse.Dirent{
			Inode: binary.BigEndian.Uint64(id[:8]),
			Type:  fuse.DT_Dir,
			Name:  snapshot.Time.Format(time.RFC3339),
		})
	}

	return ret, nil
}

func (sn *snapshots) Lookup(ctx context.Context, name string) (fs.Node, error) {
	// This is kind of lame: we reload each snapshot and check the name
	// (which is the timestamp)
	for id := range sn.repo.List(backend.Snapshot, ctx.Done()) {
		snapshot, err := restic.LoadSnapshot(sn.repo, id)
		if err != nil {
			return nil, err
		}
		if snapshot.Time.Format(time.RFC3339) == name {
			tree, err := restic.LoadTree(sn.repo, snapshot.Tree)
			if err != nil {
				return nil, err
			}
			return &dir{
				repo: sn.repo,
				tree: tree,
			}, nil
		}
	}
	return nil, fuse.ENOENT
}

var _ = fs.HandleReadDirAller(&dir{})
var _ = fs.NodeStringLookuper(&dir{})

type dir struct {
	repo  *repository.Repository
	tree  *restic.Tree
	inode uint64
}

func (d *dir) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = d.inode
	a.Mode = os.ModeDir | 0555
	return nil
}

func (d *dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	ret := make([]fuse.Dirent, 0, len(d.tree.Nodes))

	for _, node := range d.tree.Nodes {
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
	for _, node := range d.tree.Nodes {
		if node.Name == name {
			switch {
			case node.Mode.IsDir():
				subtree, err := restic.LoadTree(d.repo, node.Subtree)
				if err != nil {
					return nil, err
				}
				return &dir{
					repo:  d.repo,
					tree:  subtree,
					inode: binary.BigEndian.Uint64(node.Subtree[:8]),
				}, nil
			case node.Mode.IsRegular():
				return makeFile(d.repo, node)
			}
		}
	}

	return nil, fuse.ENOENT
}

var _ = fs.HandleReader(&file{})

type file struct {
	repo *repository.Repository
	node *restic.Node

	sizes []uint32

	// cleartext contents
	clearContent [][]byte
}

func makeFile(repo *repository.Repository, node *restic.Node) (*file, error) {
	sizes := make([]uint32, len(node.Content))
	for i, bid := range node.Content {
		_, _, _, length, err := repo.Index().Lookup(bid)
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
