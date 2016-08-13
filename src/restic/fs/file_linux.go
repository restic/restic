// +build linux,go1.4

package fs

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// Open opens a file for reading, without updating the atime and without caching data on read.
func Open(name string) (File, error) {
	file, err := os.OpenFile(name, os.O_RDONLY|syscall.O_NOATIME, 0)
	if os.IsPermission(err) {
		file, err = os.OpenFile(name, os.O_RDONLY, 0)
	}
	return &nonCachingFile{File: file}, err
}

// these constants should've been defined in x/sys/unix, but somehow aren't.
const (
	_POSIX_FADV_NORMAL = iota
	_POSIX_FADV_RANDOM
	_POSIX_FADV_SEQUENTIAL
	_POSIX_FADV_WILLNEED
	_POSIX_FADV_DONTNEED
	_POSIX_FADV_NOREUSE
)

// nonCachingFile wraps an *os.File and calls fadvise() to instantly forget
// data that has been read or written.
type nonCachingFile struct {
	*os.File
	readOffset int64
}

func (f *nonCachingFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)

	if n > 0 {
		ferr := unix.Fadvise(int(f.File.Fd()), f.readOffset, int64(n), _POSIX_FADV_DONTNEED)

		f.readOffset += int64(n)

		if err == nil {
			err = ferr
		}
	}

	return n, err
}

// ClearCache syncs and then removes the file's content from the OS cache.
func ClearCache(file File) error {
	f, ok := file.(*os.File)
	if !ok {
		panic("ClearCache called for file not *os.File")
	}

	err := f.Sync()
	if err != nil {
		return err
	}

	return unix.Fadvise(int(f.Fd()), 0, 0, _POSIX_FADV_DONTNEED)
}

// Chmod changes the mode of the named file to mode.
func Chmod(name string, mode os.FileMode) error {
	return os.Chmod(fixpath(name), mode)
}

// Mkdir creates a new directory with the specified name and permission bits.
// If there is an error, it will be of type *PathError.
func Mkdir(name string, perm os.FileMode) error {
	return os.Mkdir(name, perm)
}

// MkdirAll creates a directory named path,
// along with any necessary parents, and returns nil,
// or else returns an error.
// The permission bits perm are used for all
// directories that MkdirAll creates.
// If path is already a directory, MkdirAll does nothing
// and returns nil.
func MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// Readlink returns the destination of the named symbolic link.
// If there is an error, it will be of type *PathError.
func Readlink(name string) (string, error) {
	return os.Readlink(name)
}

// Remove removes the named file or directory.
// If there is an error, it will be of type *PathError.
func Remove(name string) error {
	return os.Remove(name)
}

// RemoveAll removes path and any children it contains.
// It removes everything it can but returns the first error
// it encounters.  If the path does not exist, RemoveAll
// returns nil (no error).
func RemoveAll(path string) error {
	return os.RemoveAll(path)
}

// Rename renames (moves) oldpath to newpath.
// If newpath already exists, Rename replaces it.
// OS-specific restrictions may apply when oldpath and newpath are in different directories.
// If there is an error, it will be of type *LinkError.
func Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

// Symlink creates newname as a symbolic link to oldname.
// If there is an error, it will be of type *LinkError.
func Symlink(oldname, newname string) error {
	return os.Symlink(oldname, newname)
}

// Stat returns a FileInfo structure describing the named file.
// If there is an error, it will be of type *PathError.
func Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

// Lstat returns the FileInfo structure describing the named file.
// If the file is a symbolic link, the returned FileInfo
// describes the symbolic link.  Lstat makes no attempt to follow the link.
// If there is an error, it will be of type *PathError.
func Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(name)
}

// Create creates the named file with mode 0666 (before umask), truncating
// it if it already exists. If successful, methods on the returned
// File can be used for I/O; the associated file descriptor has mode
// O_RDWR.
// If there is an error, it will be of type *PathError.
func Create(name string) (*os.File, error) {
	return os.Create(name)
}

// OpenFile is the generalized open call; most users will use Open
// or Create instead.  It opens the named file with specified flag
// (O_RDONLY etc.) and perm, (0666 etc.) if applicable.  If successful,
// methods on the returned File can be used for I/O.
// If there is an error, it will be of type *PathError.
func OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}

// Walk walks the file tree rooted at root, calling walkFn for each file or
// directory in the tree, including root. All errors that arise visiting files
// and directories are filtered by walkFn. The files are walked in lexical
// order, which makes the output deterministic but means that for very
// large directories Walk can be inefficient.
// Walk does not follow symbolic links.
func Walk(root string, walkFn filepath.WalkFunc) error {
	return filepath.Walk(root, walkFn)
}
