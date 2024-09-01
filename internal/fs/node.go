package fs

import (
	"os"
	"os/user"
	"strconv"
	"sync"
	"syscall"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

// NodeFromFileInfo returns a new node from the given path and FileInfo. It
// returns the first error that is encountered, together with a node.
func NodeFromFileInfo(path string, fi os.FileInfo, ignoreXattrListError bool) (*restic.Node, error) {
	mask := os.ModePerm | os.ModeType | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	node := &restic.Node{
		Path:    path,
		Name:    fi.Name(),
		Mode:    fi.Mode() & mask,
		ModTime: fi.ModTime(),
	}

	node.Type = nodeTypeFromFileInfo(fi)
	if node.Type == restic.NodeTypeFile {
		node.Size = uint64(fi.Size())
	}

	err := nodeFillExtra(node, path, fi, ignoreXattrListError)
	return node, err
}

func nodeTypeFromFileInfo(fi os.FileInfo) restic.NodeType {
	switch fi.Mode() & os.ModeType {
	case 0:
		return restic.NodeTypeFile
	case os.ModeDir:
		return restic.NodeTypeDir
	case os.ModeSymlink:
		return restic.NodeTypeSymlink
	case os.ModeDevice | os.ModeCharDevice:
		return restic.NodeTypeCharDev
	case os.ModeDevice:
		return restic.NodeTypeDev
	case os.ModeNamedPipe:
		return restic.NodeTypeFifo
	case os.ModeSocket:
		return restic.NodeTypeSocket
	case os.ModeIrregular:
		return restic.NodeTypeIrregular
	}

	return restic.NodeTypeInvalid
}

func nodeFillExtra(node *restic.Node, path string, fi os.FileInfo, ignoreXattrListError bool) error {
	if fi.Sys() == nil {
		// fill minimal info with current values for uid, gid
		node.UID = uint32(os.Getuid())
		node.GID = uint32(os.Getgid())
		node.ChangeTime = node.ModTime
		return nil
	}

	stat := ExtendedStat(fi)

	node.Inode = stat.Inode
	node.DeviceID = stat.DeviceID
	node.ChangeTime = stat.ChangeTime
	node.AccessTime = stat.AccessTime

	node.UID = stat.UID
	node.GID = stat.GID
	node.User = lookupUsername(stat.UID)
	node.Group = lookupGroup(stat.GID)

	switch node.Type {
	case restic.NodeTypeFile:
		node.Size = uint64(stat.Size)
		node.Links = stat.Links
	case restic.NodeTypeDir:
	case restic.NodeTypeSymlink:
		var err error
		node.LinkTarget, err = os.Readlink(fixpath(path))
		node.Links = stat.Links
		if err != nil {
			return errors.WithStack(err)
		}
	case restic.NodeTypeDev:
		node.Device = stat.Device
		node.Links = stat.Links
	case restic.NodeTypeCharDev:
		node.Device = stat.Device
		node.Links = stat.Links
	case restic.NodeTypeFifo:
	case restic.NodeTypeSocket:
	default:
		return errors.Errorf("unsupported file type %q", node.Type)
	}

	allowExtended, err := nodeFillGenericAttributes(node, path, &stat)
	if allowExtended {
		// Skip processing ExtendedAttributes if allowExtended is false.
		err = errors.CombineErrors(err, nodeFillExtendedAttributes(node, path, ignoreXattrListError))
	}
	return err
}

var (
	uidLookupCache      = make(map[uint32]string)
	uidLookupCacheMutex = sync.RWMutex{}
)

// Cached user name lookup by uid. Returns "" when no name can be found.
func lookupUsername(uid uint32) string {
	uidLookupCacheMutex.RLock()
	username, ok := uidLookupCache[uid]
	uidLookupCacheMutex.RUnlock()

	if ok {
		return username
	}

	u, err := user.LookupId(strconv.Itoa(int(uid)))
	if err == nil {
		username = u.Username
	}

	uidLookupCacheMutex.Lock()
	uidLookupCache[uid] = username
	uidLookupCacheMutex.Unlock()

	return username
}

var (
	gidLookupCache      = make(map[uint32]string)
	gidLookupCacheMutex = sync.RWMutex{}
)

// Cached group name lookup by gid. Returns "" when no name can be found.
func lookupGroup(gid uint32) string {
	gidLookupCacheMutex.RLock()
	group, ok := gidLookupCache[gid]
	gidLookupCacheMutex.RUnlock()

	if ok {
		return group
	}

	g, err := user.LookupGroupId(strconv.Itoa(int(gid)))
	if err == nil {
		group = g.Name
	}

	gidLookupCacheMutex.Lock()
	gidLookupCache[gid] = group
	gidLookupCacheMutex.Unlock()

	return group
}

// NodeCreateAt creates the node at the given path but does NOT restore node meta data.
func NodeCreateAt(node *restic.Node, path string) error {
	debug.Log("create node %v at %v", node.Name, path)

	switch node.Type {
	case restic.NodeTypeDir:
		if err := nodeCreateDirAt(node, path); err != nil {
			return err
		}
	case restic.NodeTypeFile:
		if err := nodeCreateFileAt(path); err != nil {
			return err
		}
	case restic.NodeTypeSymlink:
		if err := nodeCreateSymlinkAt(node, path); err != nil {
			return err
		}
	case restic.NodeTypeDev:
		if err := nodeCreateDevAt(node, path); err != nil {
			return err
		}
	case restic.NodeTypeCharDev:
		if err := nodeCreateCharDevAt(node, path); err != nil {
			return err
		}
	case restic.NodeTypeFifo:
		if err := nodeCreateFifoAt(path); err != nil {
			return err
		}
	case restic.NodeTypeSocket:
		return nil
	default:
		return errors.Errorf("filetype %q not implemented", node.Type)
	}

	return nil
}

func nodeCreateDirAt(node *restic.Node, path string) error {
	err := os.Mkdir(fixpath(path), node.Mode)
	if err != nil && !os.IsExist(err) {
		return errors.WithStack(err)
	}

	return nil
}

func nodeCreateFileAt(path string) error {
	f, err := OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return errors.WithStack(err)
	}

	if err := f.Close(); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func nodeCreateSymlinkAt(node *restic.Node, path string) error {
	if err := os.Symlink(node.LinkTarget, fixpath(path)); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func nodeCreateDevAt(node *restic.Node, path string) error {
	return mknod(path, syscall.S_IFBLK|0600, node.Device)
}

func nodeCreateCharDevAt(node *restic.Node, path string) error {
	return mknod(path, syscall.S_IFCHR|0600, node.Device)
}

func nodeCreateFifoAt(path string) error {
	return mkfifo(path, 0600)
}

func mkfifo(path string, mode uint32) (err error) {
	return mknod(path, mode|syscall.S_IFIFO, 0)
}

// NodeRestoreMetadata restores node metadata
func NodeRestoreMetadata(node *restic.Node, path string, warn func(msg string)) error {
	err := nodeRestoreMetadata(node, path, warn)
	if err != nil {
		// It is common to have permission errors for folders like /home
		// unless you're running as root, so ignore those.
		if os.Geteuid() > 0 && errors.Is(err, os.ErrPermission) {
			debug.Log("not running as root, ignoring permission error for %v: %v",
				path, err)
			return nil
		}
		debug.Log("restoreMetadata(%s) error %v", path, err)
	}

	return err
}

func nodeRestoreMetadata(node *restic.Node, path string, warn func(msg string)) error {
	var firsterr error

	if err := lchown(path, int(node.UID), int(node.GID)); err != nil {
		firsterr = errors.WithStack(err)
	}

	if err := nodeRestoreExtendedAttributes(node, path); err != nil {
		debug.Log("error restoring extended attributes for %v: %v", path, err)
		if firsterr == nil {
			firsterr = err
		}
	}

	if err := nodeRestoreGenericAttributes(node, path, warn); err != nil {
		debug.Log("error restoring generic attributes for %v: %v", path, err)
		if firsterr == nil {
			firsterr = err
		}
	}

	if err := nodeRestoreTimestamps(node, path); err != nil {
		debug.Log("error restoring timestamps for %v: %v", path, err)
		if firsterr == nil {
			firsterr = err
		}
	}

	// Moving RestoreTimestamps and restoreExtendedAttributes calls above as for readonly files in windows
	// calling Chmod below will no longer allow any modifications to be made on the file and the
	// calls above would fail.
	if node.Type != restic.NodeTypeSymlink {
		if err := chmod(path, node.Mode); err != nil {
			if firsterr == nil {
				firsterr = errors.WithStack(err)
			}
		}
	}

	return firsterr
}

func nodeRestoreTimestamps(node *restic.Node, path string) error {
	var utimes = [...]syscall.Timespec{
		syscall.NsecToTimespec(node.AccessTime.UnixNano()),
		syscall.NsecToTimespec(node.ModTime.UnixNano()),
	}

	if node.Type == restic.NodeTypeSymlink {
		return nodeRestoreSymlinkTimestamps(path, utimes)
	}

	if err := syscall.UtimesNano(path, utimes[:]); err != nil {
		return errors.Wrap(err, "UtimesNano")
	}

	return nil
}
