// +build linux,go1.4

package fs

import (
	"os"
	"syscall"

	"restic/errors"

	"golang.org/x/sys/unix"
)

// Open opens a file for reading, without updating the atime and without caching data on read.
func Open(name string) (File, error) {
	file, err := os.OpenFile(name, os.O_RDONLY|syscall.O_NOATIME, 0)
	if os.IsPermission(errors.Cause(err)) {
		file, err = os.OpenFile(name, os.O_RDONLY, 0)
	}
	return &nonCachingFile{File: file}, err
}

// nonCachingFile wraps an *os.File and calls fadvise() to instantly forget
// data that has been read or written.
type nonCachingFile struct {
	*os.File
	readOffset int64
}

func (f *nonCachingFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)

	if n > 0 {
		ferr := unix.Fadvise(int(f.File.Fd()), f.readOffset, int64(n), unix.FADV_DONTNEED)

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

	return unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_DONTNEED)
}
