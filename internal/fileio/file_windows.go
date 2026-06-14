package fileio

import (
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

// TempFile creates a temporary file which is marked as delete-on-close
func TempFile(dir, prefix string) (f *os.File, err error) {
	// slightly modified implementation of os.CreateTemp(dir, prefix) to allow us to add
	// the FILE_ATTRIBUTE_TEMPORARY | FILE_FLAG_DELETE_ON_CLOSE flags.
	// These provide two large benefits:
	// FILE_ATTRIBUTE_TEMPORARY tells Windows to keep the file in memory only if possible
	// which reduces the amount of unnecessary disk writes.
	// FILE_FLAG_DELETE_ON_CLOSE instructs Windows to automatically delete the file once
	// all file descriptors are closed.

	if dir == "" {
		dir = os.TempDir()
	}

	access := uint32(windows.GENERIC_READ | windows.GENERIC_WRITE)
	creation := uint32(windows.CREATE_NEW)
	share := uint32(0) // prevent other processes from accessing the file
	flags := uint32(windows.FILE_ATTRIBUTE_TEMPORARY | windows.FILE_FLAG_DELETE_ON_CLOSE)

	for i := 0; i < 10000; i++ {
		randSuffix := strconv.Itoa(int(1e9 + rand.Intn(1e9)%1e9))[1:]
		path := filepath.Join(dir, prefix+randSuffix)

		ptr, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return nil, err
		}
		h, err := windows.CreateFile(ptr, access, share, nil, creation, flags, 0)
		if os.IsExist(err) {
			continue
		}
		// Access denied error can occur if the tmp files conflict with each other.
		if isAccessDeniedError(err) {
			continue
		}
		return os.NewFile(uintptr(h), path), err
	}

	// Proper error handling is still to do
	return nil, os.ErrExist
}

func isAccessDeniedError(err error) bool {
	if errno, ok := err.(syscall.Errno); ok {
		return errno == windows.ERROR_ACCESS_DENIED
	}
	return false
}
