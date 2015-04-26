package restic

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"github.com/restic/restic/debug"
)

func (node *Node) OpenForReading() (*os.File, error) {
	file, err := os.OpenFile(node.path, os.O_RDONLY|syscall.O_NOATIME, 0)
	if os.IsPermission(err) {
		return os.OpenFile(node.path, os.O_RDONLY, 0)
	}
	return file, err
}

func (node *Node) fillExtra(path string, fi os.FileInfo) error {
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}

	node.ChangeTime = time.Unix(stat.Ctim.Unix())
	node.AccessTime = time.Unix(stat.Atim.Unix())
	node.UID = stat.Uid
	node.GID = stat.Gid

	// TODO: cache uid lookup
	if u, err := user.LookupId(strconv.Itoa(int(stat.Uid))); err == nil {
		node.User = u.Username
	}

	// TODO: implement getgrnam() or use https://github.com/kless/osutil
	// if g, nil := user.LookupId(strconv.Itoa(int(stat.Uid))); err == nil {
	// 	node.User = u.Username
	// }

	node.Inode = stat.Ino

	var err error

	switch node.Type {
	case "file":
		node.Size = uint64(stat.Size)
		node.Links = uint64(stat.Nlink)
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

func (node *Node) createDevAt(path string) error {
	return syscall.Mknod(path, syscall.S_IFBLK|0600, int(node.Device))
}

func (node *Node) createCharDevAt(path string) error {
	return syscall.Mknod(path, syscall.S_IFCHR|0600, int(node.Device))
}

func (node *Node) createFifoAt(path string) error {
	return syscall.Mkfifo(path, 0600)
}

func (node *Node) isNewer(path string, fi os.FileInfo) bool {
	// if this node has a type other than "file", treat as if content has changed
	if node.Type != "file" {
		debug.Log("node.isNewer", "node %v is newer: not file", path)
		return true
	}

	// if the name or type has changed, this is surely something different
	tpe := nodeTypeFromFileInfo(path, fi)
	if node.Name != fi.Name() || node.Type != tpe {
		debug.Log("node.isNewer", "node %v is newer: name or type changed", path)
		return true
	}

	// collect extended stat
	stat := fi.Sys().(*syscall.Stat_t)

	changeTime := time.Unix(stat.Ctim.Unix())
	inode := stat.Ino
	size := uint64(stat.Size)

	// if timestamps or inodes differ, content has changed
	if node.ModTime != fi.ModTime() ||
		node.ChangeTime != changeTime ||
		node.Inode != inode ||
		node.Size != size {
		debug.Log("node.isNewer", "node %v is newer: timestamp, size or inode changed", path)
		return true
	}

	// otherwise the node is assumed to have the same content
	debug.Log("node.isNewer", "node %v is not newer", path)
	return false
}
