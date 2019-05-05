package restic

import (
	"syscall"

	"github.com/restic/restic/internal/errors"
)

// mknod() creates a filesystem node (file, device
// special file, or named pipe) named pathname, with attributes
// specified by mode and dev.
var mknod = func(path string, mode uint32, dev int) (err error) {
	return errors.New("device nodes cannot be created on windows")
}

// Windows doesn't need lchown
var lchown = func(path string, uid int, gid int) (err error) {
	return nil
}

func (node Node) restoreSymlinkTimestamps(path string, utimes [2]syscall.Timespec) error {
	return nil
}

func (node Node) device() int {
	return int(node.Device)
}

// Getxattr retrieves extended attribute data associated with path.
func Getxattr(path, name string) ([]byte, error) {
	return nil, nil
}

// Listxattr retrieves a list of names of extended attributes associated with the
// given path in the file system.
func Listxattr(path string) ([]string, error) {
	return nil, nil
}

// Setxattr associates name and data together as an attribute of path.
func Setxattr(path, name string, data []byte) error {
	return nil
}

type statWin syscall.Win32FileAttributeData

//ToStatT call the Windows system call Win32FileAttributeData.
func toStatT(i interface{}) (statT, bool) {
	if i == nil {
		return nil, false
	}
	s, ok := i.(*syscall.Win32FileAttributeData)
	if ok && s != nil {
		return statWin(*s), true
	}
	return nil, false
}

func (s statWin) dev() uint64   { return 0 }
func (s statWin) ino() uint64   { return 0 }
func (s statWin) nlink() uint64 { return 0 }
func (s statWin) uid() uint32   { return 0 }
func (s statWin) gid() uint32   { return 0 }
func (s statWin) rdev() uint64  { return 0 }

func (s statWin) size() int64 {
	return int64(s.FileSizeLow) | (int64(s.FileSizeHigh) << 32)
}

func (s statWin) atim() syscall.Timespec {
	return syscall.NsecToTimespec(s.LastAccessTime.Nanoseconds())
}

func (s statWin) mtim() syscall.Timespec {
	return syscall.NsecToTimespec(s.LastWriteTime.Nanoseconds())
}

func (s statWin) ctim() syscall.Timespec {
	// Windows does not have the concept of a "change time" in the sense Unix uses it, so we're using the LastWriteTime here.
	return syscall.NsecToTimespec(s.LastWriteTime.Nanoseconds())
}
