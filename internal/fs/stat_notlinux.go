//go:build !linux

package fs

import "os"

// Stat returns a FileInfo describing the named file. If there is an error, it
// will be of type *PathError.
func (fs Local) Stat(name string) (os.FileInfo, error) {
	return os.Stat(fixpath(name))
}

// Lstat returns the FileInfo structure describing the named file.
// If the file is a symbolic link, the returned FileInfo
// describes the symbolic link.  Lstat makes no attempt to follow the link.
// If there is an error, it will be of type *PathError.
func (fs Local) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(fixpath(name))
}
