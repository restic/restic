package restic

import (
	"path/filepath"
	"syscall"
	"unsafe"

	"restic/fs"
)

func (node Node) restoreSymlinkTimestamps(path string, utimes [2]syscall.Timespec) error {
	dir, err := fs.Open(filepath.Dir(path))
	defer dir.Close()
	if err != nil {
		return err
	}

	err = utimesNanoAt(int(dir.Fd()), filepath.Base(path), utimes, AT_SYMLINK_NOFOLLOW)

	if err != nil {
		return err
	}

	return nil
}

// very lowlevel below

const AT_SYMLINK_NOFOLLOW = 0x100

func utimensat(dirfd int, path string, times *[2]syscall.Timespec, flags int) (err error) {
	var _p0 *byte
	_p0, err = syscall.BytePtrFromString(path)
	if err != nil {
		return
	}
	_, _, e1 := syscall.Syscall6(syscall.SYS_UTIMENSAT, uintptr(dirfd), uintptr(unsafe.Pointer(_p0)), uintptr(unsafe.Pointer(times)), uintptr(flags), 0, 0)
	if e1 != 0 {
		err = e1
	}
	return
}

//sys	utimensat(dirfd int, path string, times *[2]Timespec, flags int) (err error)

func utimesNanoAt(dirfd int, path string, ts [2]syscall.Timespec, flags int) (err error) {
	return utimensat(dirfd, path, (*[2]syscall.Timespec)(unsafe.Pointer(&ts[0])), flags)
}

func (s statUnix) atim() syscall.Timespec { return s.Atim }
func (s statUnix) mtim() syscall.Timespec { return s.Mtim }
func (s statUnix) ctim() syscall.Timespec { return s.Ctim }
