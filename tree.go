package khepri

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fd0/khepri/backend"
	"github.com/juju/arrar"
)

type Tree []*Node

type Node struct {
	Name       string       `json:"name"`
	Type       string       `json:"type"`
	Mode       os.FileMode  `json:"mode,omitempty"`
	ModTime    time.Time    `json:"mtime,omitempty"`
	AccessTime time.Time    `json:"atime,omitempty"`
	ChangeTime time.Time    `json:"ctime,omitempty"`
	UID        uint32       `json:"uid"`
	GID        uint32       `json:"gid"`
	User       string       `json:"user,omitempty"`
	Group      string       `json:"group,omitempty"`
	Inode      uint64       `json:"inode,omitempty"`
	Size       uint64       `json:"size,omitempty"`
	Links      uint64       `json:"links,omitempty"`
	LinkTarget string       `json:"linktarget,omitempty"`
	Device     uint64       `json:"device,omitempty"`
	Content    []backend.ID `json:"content,omitempty"`
	Subtree    backend.ID   `json:"subtree,omitempty"`

	Tree *Tree `json:"-"`

	path string
}

type Blob struct {
	ID          backend.ID `json:"id,omitempty"`
	Size        uint64     `json:"size,omitempty"`
	Storage     backend.ID `json:"sid,omitempty"`   // encrypted ID
	StorageSize uint64     `json:"ssize,omitempty"` // encrypted Size
}

type Blobs []Blob

func (n Node) String() string {
	switch n.Type {
	case "file":
		return fmt.Sprintf("%s %5d %5d %6d %s %s",
			n.Mode, n.UID, n.GID, n.Size, n.ModTime, n.Name)
	case "dir":
		return fmt.Sprintf("%s %5d %5d %6d %s %s",
			n.Mode|os.ModeDir, n.UID, n.GID, n.Size, n.ModTime, n.Name)
	}

	return fmt.Sprintf("<Node(%s) %s>", n.Type, n.Name)
}

func (t Tree) String() string {
	s := []string{}
	for _, n := range t {
		s = append(s, n.String())
	}
	return strings.Join(s, "\n")
}

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

	// TODO: implement getgrnam() or use https://github.com/kless/osutil
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
	node := GetNode()
	node.path = path
	node.Name = fi.Name()
	node.Mode = fi.Mode() & os.ModePerm
	node.ModTime = fi.ModTime()

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

func (node *Node) CreateAt(ch *ContentHandler, path string) error {
	switch node.Type {
	case "dir":
		err := os.Mkdir(path, node.Mode)
		if err != nil {
			return arrar.Annotate(err, "Mkdir")
		}

		err = os.Lchown(path, int(node.UID), int(node.GID))
		if err != nil {
			return arrar.Annotate(err, "Lchown")
		}

		var utimes = []syscall.Timespec{
			syscall.NsecToTimespec(node.AccessTime.UnixNano()),
			syscall.NsecToTimespec(node.ModTime.UnixNano()),
		}
		err = syscall.UtimesNano(path, utimes)
		if err != nil {
			return arrar.Annotate(err, "Utimesnano")
		}
	case "file":
		// TODO: handle hard links
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
		defer f.Close()
		if err != nil {
			return arrar.Annotate(err, "OpenFile")
		}

		for _, blobid := range node.Content {
			buf, err := ch.Load(backend.Data, blobid)
			if err != nil {
				return arrar.Annotate(err, "Load")
			}

			_, err = f.Write(buf)
			if err != nil {
				return arrar.Annotate(err, "Write")
			}
		}

		f.Close()

		err = os.Lchown(path, int(node.UID), int(node.GID))
		if err != nil {
			return arrar.Annotate(err, "Lchown")
		}

		var utimes = []syscall.Timespec{
			syscall.NsecToTimespec(node.AccessTime.UnixNano()),
			syscall.NsecToTimespec(node.ModTime.UnixNano()),
		}
		err = syscall.UtimesNano(path, utimes)
		if err != nil {
			return arrar.Annotate(err, "Utimesnano")
		}
	case "symlink":
		err := os.Symlink(node.LinkTarget, path)
		if err != nil {
			return arrar.Annotate(err, "Symlink")
		}

		err = os.Lchown(path, int(node.UID), int(node.GID))
		if err != nil {
			return arrar.Annotate(err, "Lchown")
		}

		f, err := os.OpenFile(path, O_PATH|syscall.O_NOFOLLOW, 0600)
		defer f.Close()
		if err != nil {
			return arrar.Annotate(err, "OpenFile")
		}

		// TODO: Get Futimes() working on older Linux kernels (fails with 3.2.0)
		// var utimes = []syscall.Timeval{
		// 	syscall.NsecToTimeval(node.AccessTime.UnixNano()),
		// 	syscall.NsecToTimeval(node.ModTime.UnixNano()),
		// }
		// err = syscall.Futimes(int(f.Fd()), utimes)
		// if err != nil {
		// 	return arrar.Annotate(err, "Futimes")
		// }

		return nil
	case "dev":
		err := syscall.Mknod(path, syscall.S_IFBLK|0600, int(node.Device))
		if err != nil {
			return arrar.Annotate(err, "Mknod")
		}
	case "chardev":
		err := syscall.Mknod(path, syscall.S_IFCHR|0600, int(node.Device))
		if err != nil {
			return arrar.Annotate(err, "Mknod")
		}
	case "fifo":
		err := syscall.Mkfifo(path, 0600)
		if err != nil {
			return arrar.Annotate(err, "Mkfifo")
		}
	case "socket":
		// nothing to do, we do not restore sockets
	default:
		return fmt.Errorf("filetype %q not implemented!\n", node.Type)
	}

	err := os.Chmod(path, node.Mode)
	if err != nil {
		return arrar.Annotate(err, "Chmod")
	}

	err = os.Chown(path, int(node.UID), int(node.GID))
	if err != nil {
		return arrar.Annotate(err, "Chown")
	}

	err = os.Chtimes(path, node.AccessTime, node.ModTime)
	if err != nil {
		return arrar.Annotate(err, "Chtimes")
	}

	return nil
}

func (b Blob) Free() {
	if b.ID != nil {
		b.ID.Free()
	}

	if b.Storage != nil {
		b.Storage.Free()
	}
}
