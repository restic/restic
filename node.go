package restic

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/juju/arrar"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/server"
)

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
	Content    []backend.ID `json:"content"`
	Subtree    backend.ID   `json:"subtree,omitempty"`

	Error string `json:"error,omitempty"`

	tree *Tree

	path  string
	err   error
	blobs server.Blobs
}

func (node Node) String() string {
	switch node.Type {
	case "file":
		return fmt.Sprintf("%s %5d %5d %6d %s %s",
			node.Mode, node.UID, node.GID, node.Size, node.ModTime, node.Name)
	case "dir":
		return fmt.Sprintf("%s %5d %5d %6d %s %s",
			node.Mode|os.ModeDir, node.UID, node.GID, node.Size, node.ModTime, node.Name)
	}

	return fmt.Sprintf("<Node(%s) %s>", node.Type, node.Name)
}

func (node Node) Tree() *Tree {
	return node.tree
}

func NodeFromFileInfo(path string, fi os.FileInfo) (*Node, error) {
	node := &Node{
		path:    path,
		Name:    fi.Name(),
		Mode:    fi.Mode() & (os.ModePerm | os.ModeType),
		ModTime: fi.ModTime(),
	}

	node.Type = nodeTypeFromFileInfo(fi)
	if node.Type == "file" {
		node.Size = uint64(fi.Size())
	}

	err := node.fillExtra(path, fi)
	return node, err
}

func nodeTypeFromFileInfo(fi os.FileInfo) string {
	switch fi.Mode() & (os.ModeType | os.ModeCharDevice) {
	case 0:
		return "file"
	case os.ModeDir:
		return "dir"
	case os.ModeSymlink:
		return "symlink"
	case os.ModeDevice | os.ModeCharDevice:
		return "chardev"
	case os.ModeDevice:
		return "dev"
	case os.ModeNamedPipe:
		return "fifo"
	case os.ModeSocket:
		return "socket"
	}

	return ""
}

func (node *Node) CreateAt(path string, m *Map, s *server.Server) error {
	switch node.Type {
	case "dir":
		if err := node.createDirAt(path); err != nil {
			return err
		}
	case "file":
		if err := node.createFileAt(path, m, s); err != nil {
			return err
		}
	case "symlink":
		if err := node.createSymlinkAt(path); err != nil {
			return err
		}
	case "dev":
		if err := node.createDevAt(path); err != nil {
			return arrar.Annotate(err, "Mknod")
		}
	case "chardev":
		if err := node.createCharDevAt(path); err != nil {
			return arrar.Annotate(err, "Mknod")
		}
	case "fifo":
		if err := node.createFifoAt(path); err != nil {
			return arrar.Annotate(err, "Mkfifo")
		}
	case "socket":
		return nil
	default:
		return fmt.Errorf("filetype %q not implemented!\n", node.Type)
	}

	return node.restoreMetadata(path)
}

func (node Node) restoreMetadata(path string) error {
	var err error

	err = os.Lchown(path, int(node.UID), int(node.GID))
	if err != nil {
		return arrar.Annotate(err, "Lchown")
	}

	if node.Type == "symlink" {
		return nil
	}

	err = os.Chmod(path, node.Mode)
	if err != nil {
		return arrar.Annotate(err, "Chmod")
	}

	var utimes = []syscall.Timespec{
		syscall.NsecToTimespec(node.AccessTime.UnixNano()),
		syscall.NsecToTimespec(node.ModTime.UnixNano()),
	}
	err = syscall.UtimesNano(path, utimes)
	if err != nil {
		return arrar.Annotate(err, "Utimesnano")
	}

	return nil
}

func (node Node) createDirAt(path string) error {
	err := os.Mkdir(path, node.Mode)
	if err != nil {
		return arrar.Annotate(err, "Mkdir")
	}

	return nil
}

func (node Node) createFileAt(path string, m *Map, s *server.Server) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	defer f.Close()

	if err != nil {
		return arrar.Annotate(err, "OpenFile")
	}

	for _, blobid := range node.Content {
		blob, err := m.FindID(blobid)
		if err != nil {
			return arrar.Annotate(err, "Find Blob")
		}

		buf, err := s.Load(backend.Data, blob)
		if err != nil {
			return arrar.Annotate(err, "Load")
		}

		_, err = f.Write(buf)
		if err != nil {
			return arrar.Annotate(err, "Write")
		}
	}

	return nil
}

func (node Node) createSymlinkAt(path string) error {
	err := os.Symlink(node.LinkTarget, path)
	if err != nil {
		return arrar.Annotate(err, "Symlink")
	}

	return nil
}

func (node Node) MarshalJSON() ([]byte, error) {
	type nodeJSON Node
	nj := nodeJSON(node)
	name := strconv.Quote(node.Name)
	nj.Name = name[1 : len(name)-1]

	return json.Marshal(nj)
}

func (node *Node) UnmarshalJSON(data []byte) error {
	type nodeJSON Node
	nj := (*nodeJSON)(node)

	err := json.Unmarshal(data, nj)
	if err != nil {
		return err
	}

	nj.Name, err = strconv.Unquote(`"` + nj.Name + `"`)
	return err
}

func (node Node) Equals(other Node) bool {
	if node.Name != other.Name {
		return false
	}
	if node.Type != other.Type {
		return false
	}
	if node.Mode != other.Mode {
		return false
	}
	if node.ModTime != other.ModTime {
		return false
	}
	if node.AccessTime != other.AccessTime {
		return false
	}
	if node.ChangeTime != other.ChangeTime {
		return false
	}
	if node.UID != other.UID {
		return false
	}
	if node.GID != other.GID {
		return false
	}
	if node.User != other.User {
		return false
	}
	if node.Group != other.Group {
		return false
	}
	if node.Inode != other.Inode {
		return false
	}
	if node.Size != other.Size {
		return false
	}
	if node.Links != other.Links {
		return false
	}
	if node.LinkTarget != other.LinkTarget {
		return false
	}
	if node.Device != other.Device {
		return false
	}
	if !node.sameContent(other) {
		return false
	}
	if !node.Subtree.Equal(other.Subtree) {
		return false
	}
	if node.Error != other.Error {
		return false
	}

	return true
}

func (node Node) sameContent(other Node) bool {
	if node.Content == nil {
		return other.Content == nil
	}

	if other.Content == nil {
		return false
	}

	if len(node.Content) != len(other.Content) {
		return false
	}

	for i := 0; i < len(node.Content); i++ {
		if !node.Content[i].Equal(other.Content[i]) {
			return false
		}
	}

	return true
}
