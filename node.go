package restic

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/juju/arrar"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/pack"
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

func (node *Node) CreateAt(path string, s *server.Server) error {
	switch node.Type {
	case "dir":
		if err := node.createDirAt(path); err != nil {
			return err
		}
	case "file":
		if err := node.createFileAt(path, s); err != nil {
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

func (node Node) createFileAt(path string, s *server.Server) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	defer f.Close()

	if err != nil {
		return arrar.Annotate(err, "OpenFile")
	}

	for _, id := range node.Content {
		buf, err := s.LoadBlob(pack.Data, id)
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

func (node *Node) isNewer(path string, fi os.FileInfo) bool {
	if node.Type != "file" {
		debug.Log("node.isNewer", "node %v is newer: not file", path)
		return true
	}

	tpe := nodeTypeFromFileInfo(fi)
	if node.Name != fi.Name() || node.Type != tpe {
		debug.Log("node.isNewer", "node %v is newer: name or type changed", path)
		return true
	}

	extendedStat := fi.Sys().(*syscall.Stat_t)
	inode := extendedStat.Ino
	size := uint64(extendedStat.Size)

	if node.ModTime != fi.ModTime() ||
		node.ChangeTime != changeTime(extendedStat) ||
		node.Inode != inode ||
		node.Size != size {
		debug.Log("node.isNewer", "node %v is newer: timestamp, size or inode changed", path)
		return true
	}

	debug.Log("node.isNewer", "node %v is not newer", path)
	return false
}

func (node *Node) fillUser(stat *syscall.Stat_t) error {
	node.UID = stat.Uid
	node.GID = stat.Gid

	username, err := lookupUsername(strconv.Itoa(int(stat.Uid)))
	if err != nil {
		return err
	}

	node.User = username
	return nil
}

var (
	uidLookupCache      = make(map[string]string)
	uidLookupCacheMutex = sync.RWMutex{}
)

func lookupUsername(uid string) (string, error) {
	uidLookupCacheMutex.RLock()
	value, ok := uidLookupCache[uid]
	uidLookupCacheMutex.RUnlock()

	if ok {
		return value, nil
	}

	u, err := user.LookupId(uid)
	if err != nil {
		return "", err
	}

	uidLookupCacheMutex.Lock()
	uidLookupCache[uid] = u.Username
	uidLookupCacheMutex.Unlock()

	return u.Username, nil
}

func (node *Node) fillExtra(path string, fi os.FileInfo) error {
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}

	node.Inode = stat.Ino

	node.fillTimes(stat)

	var err error

	if err = node.fillUser(stat); err != nil {
		return err
	}

	switch node.Type {
	case "file":
		node.Size = uint64(stat.Size)
		node.Links = uint64(stat.Nlink)
	case "dir":
	case "symlink":
		node.LinkTarget, err = os.Readlink(path)
	case "dev":
		node.Device = uint64(stat.Rdev)
	case "chardev":
		node.Device = uint64(stat.Rdev)
	case "fifo":
	case "socket":
	default:
		err = fmt.Errorf("invalid node type %q", node.Type)
	}

	return err
}
