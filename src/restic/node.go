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

	"restic/errors"

	"runtime"

	"restic/debug"
	"restic/fs"
)

// Node is a file, directory or other item in a backup.
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
	Content    IDs         `json:"content"`
	Subtree    *ID         `json:"subtree,omitempty"`

	Error string `json:"error,omitempty"`

	Path string `json:"-"`
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

// NodeFromFileInfo returns a new node from the given path and FileInfo.
func NodeFromFileInfo(path string, fi os.FileInfo) (*Node, error) {
	mask := os.ModePerm | os.ModeType | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	node := &Node{
		Path:    path,
		Name:    fi.Name(),
		Mode:    fi.Mode() & mask,
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

// CreateAt creates the node at the given path and restores all the meta data.
func (node *Node) CreateAt(path string, repo Repository) error {
	debug.Log("create node %v at %v", node.Name, path)

	switch node.Type {
	case "dir":
		if err := node.createDirAt(path); err != nil {
			return err
		}
	case "file":
		if err := node.createFileAt(path, repo); err != nil {
			return err
		}
	case "symlink":
		if err := node.createSymlinkAt(path); err != nil {
			return err
		}
	case "dev":
		if err := node.createDevAt(path); err != nil {
			return err
		}
	case "chardev":
		if err := node.createCharDevAt(path); err != nil {
			return err
		}
	case "fifo":
		if err := node.createFifoAt(path); err != nil {
			return err
		}
	case "socket":
		return nil
	default:
		return errors.Errorf("filetype %q not implemented!\n", node.Type)
	}

	err := node.restoreMetadata(path)
	if err != nil {
		debug.Log("restoreMetadata(%s) error %v", path, err)
	}

	return err
}

func (node Node) restoreMetadata(path string) error {
	var err error

	err = lchown(path, int(node.UID), int(node.GID))
	if err != nil {
		return errors.Wrap(err, "Lchown")
	}

	if node.Type != "symlink" {
		err = fs.Chmod(path, node.Mode)
		if err != nil {
			return errors.Wrap(err, "Chmod")
		}
	}

	if node.Type != "dir" {
		err = node.RestoreTimestamps(path)
		if err != nil {
			debug.Log("error restoring timestamps for dir %v: %v", path, err)
			return err
		}
	}

	return nil
}

func (node Node) RestoreTimestamps(path string) error {
	var utimes = [...]syscall.Timespec{
		syscall.NsecToTimespec(node.AccessTime.UnixNano()),
		syscall.NsecToTimespec(node.ModTime.UnixNano()),
	}

	if node.Type == "symlink" {
		return node.restoreSymlinkTimestamps(path, utimes)
	}

	if err := syscall.UtimesNano(path, utimes[:]); err != nil {
		return errors.Wrap(err, "UtimesNano")
	}

	return nil
}

func (node Node) createDirAt(path string) error {
	err := fs.Mkdir(path, node.Mode)
	if err != nil {
		return errors.Wrap(err, "Mkdir")
	}

	return nil
}

func (node Node) createFileAt(path string, repo Repository) error {
	f, err := fs.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	defer f.Close()

	if err != nil {
		return errors.Wrap(err, "OpenFile")
	}

	var buf []byte
	for _, id := range node.Content {
		size, err := repo.LookupBlobSize(id, DataBlob)
		if err != nil {
			return err
		}

		buf = buf[:cap(buf)]
		if uint(len(buf)) < size {
			buf = make([]byte, size)
		}

		n, err := repo.LoadBlob(DataBlob, id, buf)
		if err != nil {
			return err
		}
		buf = buf[:n]

		_, err = f.Write(buf)
		if err != nil {
			return errors.Wrap(err, "Write")
		}
	}

	return nil
}

func (node Node) createSymlinkAt(path string) error {
	// Windows does not allow non-admins to create soft links.
	if runtime.GOOS == "windows" {
		return nil
	}
	err := fs.Symlink(node.LinkTarget, path)
	if err != nil {
		return errors.Wrap(err, "Symlink")
	}

	return nil
}

func (node *Node) createDevAt(path string) error {
	return mknod(path, syscall.S_IFBLK|0600, int(node.Device))
}

func (node *Node) createCharDevAt(path string) error {
	return mknod(path, syscall.S_IFCHR|0600, int(node.Device))
}

func (node *Node) createFifoAt(path string) error {
	return mkfifo(path, 0600)
}

func (node Node) MarshalJSON() ([]byte, error) {
	if node.ModTime.Year() < 0 || node.ModTime.Year() > 9999 {
		err := errors.Errorf("node %v has invalid ModTime year %d: %v",
			node.Path, node.ModTime.Year(), node.ModTime)
		return nil, err
	}

	if node.ChangeTime.Year() < 0 || node.ChangeTime.Year() > 9999 {
		err := errors.Errorf("node %v has invalid ChangeTime year %d: %v",
			node.Path, node.ChangeTime.Year(), node.ChangeTime)
		return nil, err
	}

	if node.AccessTime.Year() < 0 || node.AccessTime.Year() > 9999 {
		err := errors.Errorf("node %v has invalid AccessTime year %d: %v",
			node.Path, node.AccessTime.Year(), node.AccessTime)
		return nil, err
	}

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
		return errors.Wrap(err, "Unmarshal")
	}

	nj.Name, err = strconv.Unquote(`"` + nj.Name + `"`)
	return errors.Wrap(err, "Unquote")
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
	if node.Subtree != nil {
		if other.Subtree == nil {
			return false
		}

		if !node.Subtree.Equal(*other.Subtree) {
			return false
		}
	} else {
		if other.Subtree != nil {
			return false
		}
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

// IsNewer returns true of the file has been updated since the last Stat().
func (node *Node) IsNewer(path string, fi os.FileInfo) bool {
	if node.Type != "file" {
		debug.Log("node %v is newer: not file", path)
		return true
	}

	tpe := nodeTypeFromFileInfo(fi)
	if node.Name != fi.Name() || node.Type != tpe {
		debug.Log("node %v is newer: name or type changed", path)
		return true
	}

	size := uint64(fi.Size())

	extendedStat, ok := toStatT(fi.Sys())
	if !ok {
		if node.ModTime != fi.ModTime() ||
			node.Size != size {
			debug.Log("node %v is newer: timestamp or size changed", path)
			return true
		}
		return false
	}

	inode := extendedStat.ino()

	if node.ModTime != fi.ModTime() ||
		node.ChangeTime != changeTime(extendedStat) ||
		node.Inode != uint64(inode) ||
		node.Size != size {
		debug.Log("node %v is newer: timestamp, size or inode changed", path)
		return true
	}

	debug.Log("node %v is not newer", path)
	return false
}

func (node *Node) fillUser(stat statT) error {
	node.UID = stat.uid()
	node.GID = stat.gid()

	username, err := lookupUsername(strconv.Itoa(int(stat.uid())))
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

	username := ""

	u, err := user.LookupId(uid)
	if err == nil {
		username = u.Username
	}

	uidLookupCacheMutex.Lock()
	uidLookupCache[uid] = username
	uidLookupCacheMutex.Unlock()

	return username, nil
}

func (node *Node) fillExtra(path string, fi os.FileInfo) error {
	stat, ok := toStatT(fi.Sys())
	if !ok {
		return nil
	}

	node.Inode = uint64(stat.ino())

	node.fillTimes(stat)

	var err error

	if err = node.fillUser(stat); err != nil {
		return err
	}

	switch node.Type {
	case "file":
		node.Size = uint64(stat.size())
		node.Links = uint64(stat.nlink())
	case "dir":
	case "symlink":
		node.LinkTarget, err = fs.Readlink(path)
		err = errors.Wrap(err, "Readlink")
	case "dev":
		node.Device = uint64(stat.rdev())
	case "chardev":
		node.Device = uint64(stat.rdev())
	case "fifo":
	case "socket":
	default:
		err = errors.Errorf("invalid node type %q", node.Type)
	}

	return err
}

type statT interface {
	dev() uint64
	ino() uint64
	nlink() uint64
	uid() uint32
	gid() uint32
	rdev() uint64
	size() int64
	atim() syscall.Timespec
	mtim() syscall.Timespec
	ctim() syscall.Timespec
}

func mkfifo(path string, mode uint32) (err error) {
	return mknod(path, mode|syscall.S_IFIFO, 0)
}

func (node *Node) fillTimes(stat statT) {
	ctim := stat.ctim()
	atim := stat.atim()
	node.ChangeTime = time.Unix(ctim.Unix())
	node.AccessTime = time.Unix(atim.Unix())
}

func changeTime(stat statT) time.Time {
	ctim := stat.ctim()
	return time.Unix(ctim.Unix())
}
