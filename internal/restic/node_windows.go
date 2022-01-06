package restic

import (
	"bytes"
	"encoding/gob"
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
	fileinfo, err := os.Stat(path)
	if err != nil {
		return nil, errors.Wrap(err, "Getxattr")
	}

	s, ok := fileinfo.Sys().(*syscall.Win32FileAttributeData)
	if ok && s != nil {
		if name == "CreationTime"{
			var creationTime bytes.Buffer
   			enc := gob.NewEncoder(&creationTime)
			
			if err := enc.Encode(syscall.NsecToTimespec(s.CreationTime.Nanoseconds())); err != nil {
				return nil, errors.Wrap(err, "Getxattr")
			}
   			return creationTime.Bytes(), nil
		}
		return nil, nil
	}
	return nil, nil
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
		if s.CreationTime != nil{
			return [...]string{"CreationTime"}, nil
		}
		return nil, nil
	}
	return nil, nil
}

// Setxattr associates name and data together as an attribute of path.
func Setxattr(path, name string, data []byte) error {
	pathp, e := syscall.UTF16PtrFromString(path)
	if e != nil {
		return errors.Wrap(e, "Setxattr")
	}
	h, e := syscall.CreateFile(pathp,
		syscall.FILE_WRITE_ATTRIBUTES, syscall.FILE_SHARE_WRITE, nil,
		syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if e != nil {
		return errors.Wrap(e, "Setxattr")
	}
	defer syscall.Close(h)

	var inputData bytes.Buffer
	inputData.Write(data)

	var creationTime Timespec
   	dec := gob.NewDecoder(&inputData)
	
	if err := dec.Decode(&creationTime); err != nil {
		return errors.Wrap(err, "Setxattr")
	}
   	
	c := syscall.NsecToFiletime(creationTime)
	if err := syscall.SetFileTime(h, &c, nil, nil); err != nil {
		return errors.Wrap(err, "Setxattr")
	}
	return nil
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
