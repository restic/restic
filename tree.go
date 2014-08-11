package khepri

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

type Tree struct {
	Nodes []*Node `json:"nodes,omitempty"`
}

type Node struct {
	Name       string      `json:"name"`
	Type       string      `json:"type"`
	Mode       os.FileMode `json:"mode,omitempty"`
	ModTime    time.Time   `json:"mtime,omitempty"`
	AccessTime time.Time   `json:"atime,omitempty"`
	ChangeTime time.Time   `json:"ctime,omitempty"`
	UID        uint32      `json:"uid"`
	GID        uint32      `json:"gid"`
	User       string      `json:"user,omitempty"`
	Group      string      `json:"group,omitempty"`
	Inode      uint64      `json:"inode,omitempty"`
	Size       uint64      `json:"size,omitempty"`
	Links      uint64      `json:"links,omitempty"`
	LinkTarget string      `json:"linktarget,omitempty"`
	Device     uint64      `json:"device,omitempty"`
	Content    ID          `json:"content,omitempty"`
	Subtree    ID          `json:"subtree,omitempty"`
	Tree       *Tree       `json:"-"`
}

func NewTree() *Tree {
	return &Tree{
		Nodes: []*Node{},
	}
}

func NewTreeFromPath(repo *Repository, dir string) (*Tree, error) {
	fd, err := os.Open(dir)
	defer fd.Close()
	if err != nil {
		return nil, err
	}

	entries, err := fd.Readdir(-1)
	if err != nil {
		return nil, err
	}

	tree := &Tree{
		Nodes: make([]*Node, 0, len(entries)),
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		node, err := NodeFromFileInfo(path, entry)
		if err != nil {
			return nil, err
		}

		tree.Nodes = append(tree.Nodes, node)

		if entry.IsDir() {
			node.Tree, err = NewTreeFromPath(repo, path)
			if err != nil {
				return nil, err
			}
			continue
		}

		if node.Type == "file" {
			file, err := os.Open(path)
			defer file.Close()
			if err != nil {
				return nil, err
			}

			wr, idch, err := repo.Create(TYPE_BLOB)
			if err != nil {
				return nil, err
			}

			io.Copy(wr, file)
			err = wr.Close()
			if err != nil {
				return nil, err
			}

			node.Content = <-idch
		}
	}

	return tree, nil
}

func (tree *Tree) Save(repo *Repository) (ID, error) {
	for _, node := range tree.Nodes {
		if node.Tree != nil {
			var err error
			node.Subtree, err = node.Tree.Save(repo)
			if err != nil {
				return nil, err
			}
		}
	}

	buf, err := json.Marshal(tree)
	if err != nil {
		return nil, err
	}

	wr, idch, err := repo.Create(TYPE_BLOB)
	if err != nil {
		return nil, err
	}

	_, err = wr.Write(buf)
	if err != nil {
		return nil, err
	}

	err = wr.Close()
	if err != nil {
		return nil, err
	}

	return <-idch, nil
}

func NewTreeFromRepo(repo *Repository, id ID) (*Tree, error) {
	tree := NewTree()

	rd, err := repo.Get(TYPE_BLOB, id)
	defer rd.Close()
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(rd)

	err = decoder.Decode(tree)
	if err != nil {
		return nil, err
	}

	for _, node := range tree.Nodes {
		if node.Subtree != nil {
			node.Tree, err = NewTreeFromRepo(repo, node.Subtree)
			if err != nil {
				return nil, err
			}
		}
	}

	return tree, nil
}

// TODO: make sure that node.Type is valid

func (node *Node) fill_extra(path string, fi os.FileInfo) (err error) {
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}

	node.ChangeTime = time.Unix(stat.Ctim.Unix())
	node.AccessTime = time.Unix(stat.Atim.Unix())
	node.UID = stat.Uid
	node.GID = stat.Gid

	if u, nil := user.LookupId(strconv.Itoa(int(stat.Uid))); err == nil {
		node.User = u.Username
	}

	// TODO: implement getgrnam()
	// if g, nil := user.LookupId(strconv.Itoa(int(stat.Uid))); err == nil {
	// 	node.User = u.Username
	// }

	node.Inode = stat.Ino

	switch node.Type {
	case "file":
		node.Size = uint64(stat.Size)
		node.Links = stat.Nlink
	case "dir":
		// nothing to do
	case "symlink":
		node.LinkTarget, err = os.Readlink(path)
	case "dev":
		node.Device = stat.Rdev
	case "chardev":
		node.Device = stat.Rdev
	case "fifo":
		// nothing to do
	case "socket":
		// nothing to do
	default:
		panic(fmt.Sprintf("invalid node type %q", node.Type))
	}

	return err
}

func NodeFromFileInfo(path string, fi os.FileInfo) (*Node, error) {
	node := &Node{
		Name:    fi.Name(),
		Mode:    fi.Mode() & os.ModePerm,
		ModTime: fi.ModTime(),
	}

	switch fi.Mode() & (os.ModeType | os.ModeCharDevice) {
	case 0:
		node.Type = "file"
	case os.ModeDir:
		node.Type = "dir"
	case os.ModeSymlink:
		node.Type = "symlink"
	case os.ModeDevice | os.ModeCharDevice:
		node.Type = "chardev"
	case os.ModeDevice:
		node.Type = "dev"
	case os.ModeNamedPipe:
		node.Type = "fifo"
	case os.ModeSocket:
		node.Type = "socket"
	}

	err := node.fill_extra(path, fi)
	return node, err
}
