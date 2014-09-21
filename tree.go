package khepri

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/fd0/khepri/chunker"
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
	Content    []ID        `json:"content,omitempty"`
	Subtree    ID          `json:"subtree,omitempty"`
	Tree       *Tree       `json:"-"`
	repo       *Repository
}

func NewTree() *Tree {
	return &Tree{
		Nodes: []*Node{},
	}
}

func store_chunk(repo *Repository, rd io.Reader) (ID, error) {
	data, err := ioutil.ReadAll(rd)
	if err != nil {
		return nil, err
	}

	id, err := repo.Create(TYPE_BLOB, data)
	if err != nil {
		return nil, err
	}

	return id, nil
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
		node.repo = repo

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

			if node.Size < chunker.MinSize {
				// if the file is small enough, store it directly
				id, err := store_chunk(repo, file)

				if err != nil {
					return nil, err
				}

				node.Content = []ID{id}

			} else {
				// else store chunks
				node.Content = []ID{}
				ch := chunker.New(file)

				for {
					chunk, err := ch.Next()

					if err == io.EOF {
						break
					}

					if err != nil {
						return nil, err
					}

					id, err := store_chunk(repo, bytes.NewBuffer(chunk.Data))

					node.Content = append(node.Content, id)
				}

			}
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

	data, err := json.Marshal(tree)
	if err != nil {
		return nil, err
	}

	id, err := repo.Create(TYPE_BLOB, data)
	if err != nil {
		return nil, err
	}

	return id, nil
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
		node.repo = repo

		if node.Subtree != nil {
			node.Tree, err = NewTreeFromRepo(repo, node.Subtree)
			if err != nil {
				return nil, err
			}
		}
	}

	return tree, nil
}

func (tree *Tree) CreateAt(path string) error {
	for _, node := range tree.Nodes {
		nodepath := filepath.Join(path, node.Name)

		if node.Type == "dir" {
			err := os.Mkdir(nodepath, 0700)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				continue
			}

			err = os.Chmod(nodepath, node.Mode)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				continue
			}

			err = os.Chown(nodepath, int(node.UID), int(node.GID))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				continue
			}

			err = node.Tree.CreateAt(filepath.Join(path, node.Name))
			if err != nil {
				return err
			}

			err = os.Chtimes(nodepath, node.AccessTime, node.ModTime)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				continue
			}

		} else {
			err := node.CreateAt(nodepath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				continue
			}
		}
	}

	return nil
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

func (node *Node) CreateAt(path string) error {
	if node.repo == nil {
		return fmt.Errorf("repository is nil!")
	}

	switch node.Type {
	case "file":
		// TODO: handle hard links
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
		defer f.Close()
		if err != nil {
			return err
		}

		for _, blobid := range node.Content {
			rd, err := node.repo.Get(TYPE_BLOB, blobid)
			if err != nil {
				return err
			}

			_, err = io.Copy(f, rd)
			if err != nil {
				return err
			}
		}

		f.Close()
	case "symlink":
		err := os.Symlink(node.LinkTarget, path)
		if err != nil {
			return err
		}

		err = os.Lchown(path, int(node.UID), int(node.GID))
		if err != nil {
			return err
		}

		f, err := os.OpenFile(path, O_PATH|syscall.O_NOFOLLOW, 0600)
		defer f.Close()
		if err != nil {
			return err
		}

		var utimes = []syscall.Timeval{
			syscall.NsecToTimeval(node.AccessTime.UnixNano()),
			syscall.NsecToTimeval(node.ModTime.UnixNano()),
		}
		err = syscall.Futimes(int(f.Fd()), utimes)
		if err != nil {
			return err
		}

		return nil
	case "dev":
		err := syscall.Mknod(path, syscall.S_IFBLK|0600, int(node.Device))
		if err != nil {
			return err
		}
	case "chardev":
		err := syscall.Mknod(path, syscall.S_IFCHR|0600, int(node.Device))
		if err != nil {
			return err
		}
	case "fifo":
		err := syscall.Mkfifo(path, 0600)
		if err != nil {
			return err
		}
	case "socket":
		// nothing to do, we do not restore sockets
	default:
		return fmt.Errorf("filetype %q not implemented!\n", node.Type)
	}

	err := os.Chmod(path, node.Mode)
	if err != nil {
		return err
	}

	err = os.Chown(path, int(node.UID), int(node.GID))
	if err != nil {
		return err
	}

	err = os.Chtimes(path, node.AccessTime, node.ModTime)
	if err != nil {
		return err
	}

	return nil
}
