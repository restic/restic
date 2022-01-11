package restic

import (
	"bytes"
	"encoding/binary"
	"os"
	"syscall"

	"github.com/restic/restic/internal/errors"
)

// mknod is not supported on Windows.
func mknod(path string, mode uint32, dev uint64) (err error) {
	return errors.New("device nodes cannot be created on windows")
}

// Windows doesn't need lchown
func lchown(path string, uid int, gid int) (err error) {
	return nil
}

func (node Node) restoreSymlinkTimestamps(path string, utimes [2]syscall.Timespec) error {
	return nil
}

// Getxattr retrieves extended attribute data associated with path.
func Getxattr(path, name string) ([]byte, error) {
	switch name {
	case "CreationTime":
		fileinfo, err := os.Stat(path)
		if err != nil {
			return nil, errors.Wrap(err, "Getxattr")
		}

		s, ok := fileinfo.Sys().(*syscall.Win32FileAttributeData)
		if ok && s != nil {
			var creationTime [8]byte
			binary.LittleEndian.PutUint32(creationTime[0:4], s.CreationTime.LowDateTime)
			binary.LittleEndian.PutUint32(creationTime[4:8], s.CreationTime.HighDateTime)
			return creationTime[:], nil
		}
		return nil, nil
	case "FileAttributes":
		pathp, e := syscall.UTF16PtrFromString(path)
		if e != nil {
			return nil, errors.Wrap(e, "Getxattr")
		}
		attrs, e := syscall.GetFileAttributes(pathp)
		if e != nil {
			return nil, errors.Wrap(e, "Getxattr")
		}
		fileAttributes := make([]byte, 4)
		binary.LittleEndian.PutUint32(fileAttributes, attrs)

		return fileAttributes, nil
	default:
		return nil, nil
	}
}

// Listxattr retrieves a list of names of extended attributes associated with the
// given path in the file system.
func Listxattr(path string) ([]string, error) {
	fileinfo, err := os.Stat(path)
	if err != nil {
		return nil, errors.Wrap(err, "Listxattr")
	}

	s, ok := fileinfo.Sys().(*syscall.Win32FileAttributeData)
	if ok && s != nil {
		return []string{"CreationTime", "FileAttributes"}, nil
	}
	return nil, nil
}

// Setxattr associates name and data together as an attribute of path.
func Setxattr(path, name string, data []byte) error {

	pathp, e := syscall.UTF16PtrFromString(path)
	if e != nil {
		return errors.Wrap(e, "Setxattr")
	}

	switch name {
	case "CreationTime":
		h, e := syscall.CreateFile(pathp,
			syscall.FILE_WRITE_ATTRIBUTES, syscall.FILE_SHARE_WRITE, nil,
			syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if e != nil {
			return errors.Wrap(e, "Setxattr")
		}
		defer syscall.Close(h)
	
		var inputData bytes.Buffer
		inputData.Write(data)
	
		var creationTime syscall.Filetime
		creationTime.LowDateTime = binary.LittleEndian.Uint32(data[0:4])
		creationTime.HighDateTime = binary.LittleEndian.Uint32(data[4:8])
		if err := syscall.SetFileTime(h, &creationTime, nil, nil); err != nil {
			return errors.Wrap(err, "Setxattr")
		}
		return nil
	case "FileAttributes":
		attrs := binary.LittleEndian.Uint32(data)
		if err := syscall.SetFileAttributes(pathp, attrs); err != nil {
			return errors.Wrap(err, "Setxattr")
		}
		return nil
	default:
		return nil
	}
}

type statT syscall.Win32FileAttributeData

func toStatT(i interface{}) (*statT, bool) {
	s, ok := i.(*syscall.Win32FileAttributeData)
	if ok && s != nil {
		return (*statT)(s), true
	}
	return nil, false
}

func (s statT) dev() uint64   { return 0 }
func (s statT) ino() uint64   { return 0 }
func (s statT) nlink() uint64 { return 0 }
func (s statT) uid() uint32   { return 0 }
func (s statT) gid() uint32   { return 0 }
func (s statT) rdev() uint64  { return 0 }

func (s statT) size() int64 {
	return int64(s.FileSizeLow) | (int64(s.FileSizeHigh) << 32)
}

func (s statT) atim() syscall.Timespec {
	return syscall.NsecToTimespec(s.LastAccessTime.Nanoseconds())
}

func (s statT) mtim() syscall.Timespec {
	return syscall.NsecToTimespec(s.LastWriteTime.Nanoseconds())
}

func (s statT) ctim() syscall.Timespec {
	// Windows does not have the concept of a "change time" in the sense Unix uses it, so we're using the LastWriteTime here.
	return syscall.NsecToTimespec(s.LastWriteTime.Nanoseconds())
}
