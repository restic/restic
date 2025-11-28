//go:build windows
// +build windows

package fuse

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"

	"github.com/restic/restic/internal/debug"
	"github.com/winfsp/cgofuse/fuse"
)

// windowsFSBridge implements the cgofuse.FileSystem interface and acts as a bridge
// between cgofuse and our internal, platform-agnostic FUSE implementation.
type windowsFSBridge struct {
	fuse.FileSystemBase
	root Node

	// For managing file handles (fh)
	nextFh   uint64
	fhMap    map[uint64]Handle
	fhMapMtx sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
}

// WindowsFSCanceller is an interface for the Windows FUSE filesystem to allow cancellation.
type WindowsFSCanceller interface {
	Cancel()
}

// Cancel calls the internal context.CancelFunc.
func (w *windowsFSBridge) Cancel() {
	w.cancel()
}

// openDirectory is a wrapper for a Node that represents an opened directory.
type openDirectory struct {
	handle Handle
}

// NewWindowsFS creates a new cgofuse.FileSystem instance that wraps our internal Node.
func NewWindowsFS(parentCtx context.Context, root Node) fuse.FileSystemInterface {
	ctx, cancel := context.WithCancel(parentCtx)
	return &windowsFSBridge{
		root:   root,
		nextFh: 1, // fh 0 is reserved
		fhMap:  make(map[uint64]Handle),
		ctx:    ctx,
		cancel: cancel,
	}
}

// allocateFh allocates a new file handle ID and stores the given handle.
func (w *windowsFSBridge) allocateFh(handle Handle) uint64 {
	w.fhMapMtx.Lock()
	defer w.fhMapMtx.Unlock()

	fh := w.nextFh
	w.nextFh++
	w.fhMap[fh] = handle
	return fh
}

/*
//To perserve tree cache, Forget is not called.
//It should be no problem as it only costs some Node entry buffers
func (w *windowsFSBridge) Forget(handle Handle) {
	if od, ok := handle.(openDirectory); ok {
		handle = od.handle
	}
	if forgetter, ok := handle.(NodeForgetter); ok {
		forgetter.Forget()
	}
}
*/

// releaseFh releases the file handle ID.
func (w *windowsFSBridge) releaseFh(fh uint64) {
	w.fhMapMtx.Lock()
	defer w.fhMapMtx.Unlock()
	delete(w.fhMap, fh)
}

// getHandle retrieves the handle associated with the given fh.
func (w *windowsFSBridge) getHandle(fh uint64) (Handle, bool) {
	w.fhMapMtx.Lock()
	defer w.fhMapMtx.Unlock()

	handle, ok := w.fhMap[fh]
	if ok {
		// unwraps
		if wrapper, ok := handle.(*openDirectory); ok {
			handle = wrapper.handle
		}
	}
	return handle, ok
}

// 0777 for Windows mount with backups created on unix like OS
func getExtraMode() uint32 {
	var extmode uint32 = 0
	if runtime.GOOS == "windows" {
		extmode = 0777
	}
	return extmode
}

// Statfs implements cgofuse.FileSystem.Statfs
func (w *windowsFSBridge) Statfs(path string, stat *fuse.Statfs_t) (errc int) {
	debug.Log("windowsFSBridge.Statfs: %s", path)
	// For now, provide dummy values. This might need to be more sophisticated later.
	stat.Bsize = blockSize
	stat.Frsize = blockSize
	stat.Blocks = 1024 * 1024 * 1024 / blockSize * 1024 * 4 // 4TB
	stat.Bfree = stat.Blocks / 2
	stat.Bavail = 0
	stat.Files = stat.Blocks
	stat.Ffree = stat.Files
	stat.Favail = stat.Ffree
	stat.Namemax = 4096
	return 0
}

// Getattr implements cgofuse.FileSystem.Getattr
func (w *windowsFSBridge) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	debug.Log("windowsFSBridge.Getattr: %s, fh: %d", path, fh)
	var node Node
	var err error

	if fh != ^uint64(0) { // If a valid file handle is provided
		handle, ok := w.getHandle(fh)
		if !ok {
			return -fuse.EBADF
		}
		node, ok = handle.(Node)
		if !ok {
			// If handle is not a Node, try to get it from path
			node, err = getNodeForPath(w.ctx, w.root, path)
			if err != nil {
				return -mapError(err)
			}
		}
	} else {
		node, err = getNodeForPath(w.ctx, w.root, path)
		if err != nil {
			return -mapError(err)
		}
	}

	var ourAttr Attr
	err = node.Attr(w.ctx, &ourAttr)
	if err != nil {
		return -mapError(err)
	}

	// Convert our Attr to fuse.Stat_t
	stat.Dev = 0 // Dummy value
	stat.Ino = ourAttr.Inode
	stat.Mode = FileMode2fuseMode(ourAttr.Mode) | getExtraMode()

	stat.Nlink = ourAttr.Nlink
	if stat.Nlink <= 0 {
		stat.Nlink = 1
	}
	stat.Uid = ourAttr.Uid
	stat.Gid = ourAttr.Gid
	stat.Rdev = uint64(ourAttr.Rdev)                                                              // Cast to uint64
	stat.Size = int64(ourAttr.Size)                                                               // Cast to int64
	stat.Blksize = int64(ourAttr.BlockSize)                                                       // Cast to int64
	stat.Blocks = int64(ourAttr.Blocks)                                                           // Cast to int64
	stat.Atim = fuse.Timespec{Sec: ourAttr.Atime.Unix(), Nsec: int64(ourAttr.Atime.Nanosecond())} // Cast Nsec to int64
	stat.Mtim = fuse.Timespec{Sec: ourAttr.Mtime.Unix(), Nsec: int64(ourAttr.Mtime.Nanosecond())} // Cast Nsec to int64
	stat.Ctim = fuse.Timespec{Sec: ourAttr.Ctime.Unix(), Nsec: int64(ourAttr.Ctime.Nanosecond())} // Cast Nsec to int64
	stat.Birthtim = stat.Ctim
	debug.Log("windowsFSBridge.Getattr: stat: %+v", stat)
	return 0
}

// Readdir implements cgofuse.FileSystem.Readdir
func (w *windowsFSBridge) Readdir(path string, fill func(name string, stat *fuse.Stat_t, ofst int64) bool, ofst int64, fh uint64) (errc int) {
	debug.Log("windowsFSBridge.Readdir: %s, fh: %d, ofst: %d", path, fh, ofst)

	handle, ok := w.getHandle(fh)
	if !ok {
		return -fuse.EBADF
	}

	node, ok := handle.(Node)
	if !ok {
		return -fuse.EINVAL // Should not happen if handle is valid
	}

	dirReader, ok := node.(HandleReadDirAller)
	if !ok {
		return -fuse.ENOTDIR // Not a directory handle
	}

	dirents, err := dirReader.ReadDirAll(w.ctx)
	if err != nil {
		return -mapError(err)
	}

	for i := int(ofst); i < len(dirents); i++ {
		currentOffset := int64(i)
		d := dirents[i]

		// Create a dummy stat for now, or try to get actual attributes
		// For a real implementation, we'd need to call Getattr for each entry
		// or have the Dirent contain more info.
		// For simplicity, let's just fill name and type.
		var entryStat fuse.Stat_t
		entryStat.Ino = d.Inode
		if d.Node != nil {
			entryStat.Mode = FileMode2fuseMode(d.Node.Mode) | getExtraMode()
			entryStat.Nlink = uint32(d.Node.Links)
			if entryStat.Nlink <= 0 {
				entryStat.Nlink = 1
			}
			entryStat.Uid = d.Node.UID
			entryStat.Gid = d.Node.GID
			entryStat.Rdev = uint64(d.Node.DeviceID)
			entryStat.Size = int64(d.Node.Size)
			entryStat.Blksize = blockSize
			entryStat.Blocks = int64((d.Node.Size + blockSize - 1) / blockSize)
			entryStat.Atim = fuse.Timespec{Sec: d.Node.AccessTime.Unix(), Nsec: int64(d.Node.AccessTime.Nanosecond())}
			entryStat.Mtim = fuse.Timespec{Sec: d.Node.ModTime.Unix(), Nsec: int64(d.Node.ModTime.Nanosecond())}
			entryStat.Ctim = fuse.Timespec{Sec: d.Node.ChangeTime.Unix(), Nsec: int64(d.Node.ChangeTime.Nanosecond())}
			entryStat.Birthtim = entryStat.Ctim
		} else {
			entryStat.Mode = uint32(d.Type)<<12 | 0555 | getExtraMode()
			entryStat.Nlink = 1
			entryStat.Uid = 0
			entryStat.Gid = 0
		}
		debug.Log("windowsFSBridge.Readdir: %s, entryStat: %+v", d.Name, entryStat)

		if !fill(d.Name, &entryStat, currentOffset) {
			break
		}
	}

	return 0
}

// Open implements cgofuse.FileSystem.Open
func (w *windowsFSBridge) Open(path string, flags int) (errc int, fh uint64) {
	debug.Log("windowsFSBridge.Open: %s, flags: %d", path, flags)

	node, err := getNodeForPath(w.ctx, w.root, path)
	if err != nil {
		debug.Log("getNodeForPath failed, errc: %s", err.Error())
		return -mapError(err), ^uint64(0)
	}

	opener, ok := node.(NodeOpener)
	if !ok {
		debug.Log(" node.(NodeOpener) failed")
		return -fuse.EISDIR, ^uint64(0)
	}

	// Convert cgofuse flags to our OpenRequest flags
	ourReq := OpenRequest{Flags: OpenFlags(flags)}
	var ourResp OpenResponse

	handle, err := opener.Open(w.ctx, &ourReq, &ourResp)
	if err != nil {
		debug.Log("opener.Open failed, errc: %s", err.Error())
		return -mapError(err), ^uint64(0)
	}

	newFh := w.allocateFh(handle)
	debug.Log("windowsFSBridge.Open: newFh: %d", newFh)
	return 0, newFh
}

// Release implements cgofuse.FileSystem.Release
func (w *windowsFSBridge) Release(path string, fh uint64) (errc int) {
	debug.Log("windowsFSBridge.Release: %s, fh: %d", path, fh)
	w.releaseFh(fh)
	return 0
}

// Opendir implements cgofuse.FileSystem.Opendir
func (w *windowsFSBridge) Opendir(path string) (errc int, fh uint64) {
	debug.Log("windowsFSBridge.Opendir: %s", path)

	node, err := getNodeForPath(w.ctx, w.root, path)
	if err != nil {
		debug.Log("getNodeForPath failed, errc: %s", err.Error())
		return -mapError(err), ^uint64(0)
	}

	// Wrap the node in a openDirectory and allocate a file handle
	newFh := w.allocateFh(&openDirectory{handle: node})
	debug.Log("allocateFh: newFh: %d", newFh)
	return 0, newFh
}

// Releasedir implements cgofuse.FileSystem.Releasedir
func (w *windowsFSBridge) Releasedir(path string, fh uint64) (errc int) {
	debug.Log("windowsFSBridge.Releasedir: %s, fh: %d", path, fh)
	// Delegate to Release
	return w.Release(path, fh)
}

// mapError converts a Go error to a negative syscall error code.
func mapError(err error) int {
	if err == nil {
		return 0
	}
	// Basic mapping, can be expanded
	switch {
	case os.IsNotExist(err):
		return fuse.ENOENT
	case os.IsPermission(err):
		return fuse.EACCES
	case errors.Is(err, syscall.ENOTDIR):
		return fuse.ENOTDIR
	case errors.Is(err, syscall.EISDIR):
		return fuse.EISDIR
	case errors.Is(err, syscall.EINVAL):
		return fuse.EINVAL
	default:
		debug.Log("unmapped error: %v", err)
		return fuse.EIO // Generic I/O error
	}
}

// Read implements cgofuse.FileSystem.Read
func (w *windowsFSBridge) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	debug.Log("windowsFSBridge.Read: %s, fh: %d, ofst: %d, len(buff): %d", path, fh, ofst, len(buff))

	handle, ok := w.getHandle(fh)
	if !ok {
		return -fuse.EBADF
	}

	reader, ok := handle.(HandleReader)
	if !ok {
		return -fuse.EINVAL // Not a readable handle
	}

	ourReq := ReadRequest{Offset: ofst, Size: len(buff)}
	ourResp := ReadResponse{Data: buff}

	err := reader.Read(w.ctx, &ourReq, &ourResp)
	if err != nil {
		return -mapError(err)
	}

	return len(ourResp.Data)
}

// Readlink implements cgofuse.FileSystem.Readlink
func (w *windowsFSBridge) Readlink(path string) (errc int, link string) {
	debug.Log("windowsFSBridge.Readlink: %s", path)

	node, err := getNodeForPath(w.ctx, w.root, path)
	if err != nil {
		return -mapError(err), ""
	}

	linkReader, ok := node.(NodeReadlinker)
	if !ok {
		return -fuse.EINVAL, "" // Not a symlink
	}

	target, err := linkReader.Readlink(w.ctx, &ReadlinkRequest{})
	if err != nil {
		return -mapError(err), ""
	}

	return 0, target
}

// Getxattr implements cgofuse.FileSystem.Getxattr
func (w *windowsFSBridge) Getxattr(path string, name string) (errc int, xatr []byte) {
	debug.Log("windowsFSBridge.Getxattr: %s, name: %s", path, name)

	node, err := getNodeForPath(w.ctx, w.root, path)
	if err != nil {
		return -mapError(err), nil
	}

	xattrer, ok := node.(NodeGetxattrer)
	if !ok {
		return -fuse.ENOTSUP, nil // Extended attributes not supported
	}

	ourReq := GetxattrRequest{Name: name} // Size is not needed here
	var ourResp GetxattrResponse

	err = xattrer.Getxattr(w.ctx, &ourReq, &ourResp)
	if err != nil {
		return -mapError(err), nil
	}

	return 0, ourResp.Xattr
}

// Listxattr implements cgofuse.FileSystem.Listxattr
func (w *windowsFSBridge) Listxattr(path string, fill func(name string) bool) (errc int) {
	debug.Log("windowsFSBridge.Listxattr: %s", path)

	node, err := getNodeForPath(w.ctx, w.root, path)
	if err != nil {
		return -mapError(err)
	}

	xattrer, ok := node.(NodeListxattrer)
	if !ok {
		return -fuse.ENOTSUP // Extended attributes not supported
	}

	ourReq := ListxattrRequest{} // Size is not needed here for the request
	var ourResp ListxattrResponse

	err = xattrer.Listxattr(w.ctx, &ourReq, &ourResp)
	if err != nil {
		return -mapError(err)
	}

	// ourResp.Xattr is a null-separated list of names.
	// We need to split it and call the fill function for each name.
	xattrNames := strings.Split(string(ourResp.Xattr), "\x00")
	for _, name := range xattrNames {
		if name == "" {
			continue
		}
		if !fill(name) {
			return -fuse.ERANGE // Buffer too small or fill function stopped
		}
	}

	return 0
}

// Access implements cgofuse.FileSystem.Access
func (w *windowsFSBridge) Access(path string, mask uint32) (errc int) {
	debug.Log("windowsFSBridge.Access: %s, mask: %d", path, mask)
	// For a read-only filesystem, allow read access, deny write access.
	// F_OK: check for existence
	// R_OK: check for read permission
	// W_OK: check for write permission
	// X_OK: check for execute permission

	if mask&syscall.O_RDWR != 0 || mask&syscall.O_WRONLY != 0 {
		return -fuse.EACCES // Deny write access
	}

	// Check if the file/directory exists
	_, err := getNodeForPath(w.ctx, w.root, path)
	if err != nil {
		return -mapError(err)
	}

	return 0 // Allow read and existence checks
}

// Write implements cgofuse.FileSystem.Write
func (w *windowsFSBridge) Write(path string, buff []byte, ofst int64, fh uint64) (n int) {
	debug.Log("windowsFSBridge.Write: %s, fh: %d, ofst: %d, len(buff): %d", path, fh, ofst, len(buff))
	return -fuse.EROFS // Read-only file system
}

// Mknod implements cgofuse.FileSystem.Mknod
func (w *windowsFSBridge) Mknod(path string, mode uint32, dev uint64) (errc int) {
	debug.Log("windowsFSBridge.Mknod: %s, mode: %o, dev: %d", path, mode, dev)
	return -fuse.EROFS // Read-only file system
}

// Mkdir implements cgofuse.FileSystem.Mkdir
func (w *windowsFSBridge) Mkdir(path string, mode uint32) (errc int) {
	debug.Log("windowsFSBridge.Mkdir: %s, mode: %o", path, mode)
	return -fuse.EROFS // Read-only file system
}

// Unlink implements cgofuse.FileSystem.Unlink
func (w *windowsFSBridge) Unlink(path string) (errc int) {
	debug.Log("windowsFSBridge.Unlink: %s", path)
	return -fuse.EROFS // Read-only file system
}

// Rmdir implements cgofuse.FileSystem.Rmdir
func (w *windowsFSBridge) Rmdir(path string) (errc int) {
	debug.Log("windowsFSBridge.Rmdir: %s", path)
	return -fuse.EROFS // Read-only file system
}

// Link implements cgofuse.FileSystem.Link
func (w *windowsFSBridge) Link(oldPath string, newPath string) (errc int) {
	debug.Log("windowsFSBridge.Link: old: %s, new: %s", oldPath, newPath)
	return -fuse.EROFS // Read-only file system
}

// Symlink implements cgofuse.FileSystem.Symlink
func (w *windowsFSBridge) Symlink(target string, newPath string) (errc int) {
	debug.Log("windowsFSBridge.Symlink: target: %s, new: %s", target, newPath)
	return -fuse.EROFS // Read-only file system
}

// Rename implements cgofuse.FileSystem.Rename
func (w *windowsFSBridge) Rename(oldPath string, newPath string) (errc int) {
	debug.Log("windowsFSBridge.Rename: old: %s, new: %s", oldPath, newPath)
	return -fuse.EROFS // Read-only file system
}

// Chmod implements cgofuse.FileSystem.Chmod
func (w *windowsFSBridge) Chmod(path string, mode uint32) (errc int) {
	debug.Log("windowsFSBridge.Chmod: %s, mode: %o", path, mode)
	return -fuse.EROFS // Read-only file system
}

// Chown implements cgofuse.FileSystem.Chown
func (w *windowsFSBridge) Chown(path string, uid uint32, gid uint32) (errc int) {
	debug.Log("windowsFSBridge.Chown: %s, uid: %d, gid: %d", path, uid, gid)
	return -fuse.EROFS // Read-only file system
}

// Utimens implements cgofuse.FileSystem.Utimens
func (w *windowsFSBridge) Utimens(path string, tmsp []fuse.Timespec) (errc int) {
	debug.Log("windowsFSBridge.Utimens: %s", path)
	return -fuse.EROFS // Read-only file system
}

// Setxattr implements cgofuse.FileSystem.Setxattr
func (w *windowsFSBridge) Setxattr(path string, name string, data []byte, flags int) (errc int) {
	debug.Log("windowsFSBridge.Setxattr: %s, name: %s", path, name)
	return -fuse.EROFS // Read-only file system
}

// Removexattr implements cgofuse.FileSystem.Removexattr
func (w *windowsFSBridge) Removexattr(path string, name string) (errc int) {
	debug.Log("windowsFSBridge.Removexattr: %s, name: %s", path, name)
	return -fuse.EROFS // Read-only file system
}

// Chflags implements cgofuse.FileSystem.Chflags
func (w *windowsFSBridge) Chflags(path string, flags uint32) (errc int) {
	debug.Log("windowsFSBridge.Chflags: %s, flags: %d", path, flags)
	return -fuse.EROFS // Read-only file system
}

// Setcrtime implements cgofuse.FileSystem.Setcrtime
func (w *windowsFSBridge) Setcrtime(path string, tm *fuse.Timespec) (errc int) {
	debug.Log("windowsFSBridge.Setcrtime: %s", path)
	return -fuse.EROFS // Read-only file system
}

// Setchgtime implements cgofuse.FileSystem.Setchgtime
func (w *windowsFSBridge) Setchgtime(path string, tm *fuse.Timespec) (errc int) {
	debug.Log("windowsFSBridge.Setchgtime: %s", path)
	return -fuse.EROFS // Read-only file system
}

// Truncate implements cgofuse.FileSystem.Truncate
func (w *windowsFSBridge) Truncate(path string, size int64, fh uint64) (errc int) {
	debug.Log("windowsFSBridge.Truncate: %s, size: %d, fh: %d", path, size, fh)
	return -fuse.EROFS // Read-only file system
}

// Fsync implements cgofuse.FileSystem.Fsync
func (w *windowsFSBridge) Fsync(path string, datasync bool, fh uint64) (errc int) {
	debug.Log("windowsFSBridge.Fsync: %s, datasync: %t, fh: %d", path, datasync, fh)
	return -fuse.EROFS // Read-only file system
}

// Flush implements cgofuse.FileSystem.Flush
func (w *windowsFSBridge) Flush(path string, fh uint64) (errc int) {
	debug.Log("windowsFSBridge.Flush: %s, fh: %d", path, fh)
	return 0 // No-op for read-only file system
}

// Fallocate implements cgofuse.FileSystem.Fallocate
func (w *windowsFSBridge) Fallocate(path string, mode uint32, off int64, length int64, fh uint64) (errc int) {
	debug.Log("windowsFSBridge.Fallocate: %s, mode: %d, off: %d, length: %d, fh: %d", path, mode, off, length, fh)
	return -fuse.EROFS // Read-only file system
}

// Setattr implements cgofuse.FileSystem.Setattr
func (w *windowsFSBridge) Setattr(path string, stat *fuse.Stat_t, fields uint32, fh uint64) (errc int) {
	debug.Log("windowsFSBridge.Setattr: %s, fields: %d, fh: %d", path, fields, fh)
	return -fuse.EROFS // Read-only file system
}

// Create implements cgofuse.FileSystem.Create
func (w *windowsFSBridge) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	debug.Log("windowsFSBridge.Create: %s, flags: %d, mode: %o", path, flags, mode)
	return -fuse.EROFS, ^uint64(0) // Read-only file system
}
