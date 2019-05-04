// +build !windows

package archiver

import (
	"os"
	"syscall"
	"testing"
)

type wrappedFileInfo struct {
	os.FileInfo
	sys  interface{}
	mode os.FileMode
}

func (fi wrappedFileInfo) Sys() interface{} {
	return fi.sys
}

func (fi wrappedFileInfo) Mode() os.FileMode {
	return fi.mode
}

// wrapFileInfo returns a new os.FileInfo with the mode, owner, and group fields changed.
func wrapFileInfo(t testing.TB, fi os.FileInfo, mode os.FileMode, uid, gid uint) os.FileInfo {
	// get the underlying stat_t and modify the values
	stat := fi.Sys().(*syscall.Stat_t)
	stat.Mode = uint32(mode)
	stat.Uid = uint32(uid)
	stat.Gid = uint32(gid)

	// wrap the os.FileInfo so we can return a modified stat_t
	res := wrappedFileInfo{
		FileInfo: fi,
		sys:      stat,
		mode:     mode,
	}

	return res
}
