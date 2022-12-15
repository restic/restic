package fs

import (
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// fixpath returns an absolute path on windows, so restic can open long file
// names.
func fixpath(name string) string {
	abspath, err := filepath.Abs(name)
	if err == nil {
		// Check if \\?\UNC\ already exist
		if strings.HasPrefix(abspath, `\\?\UNC\`) {
			return abspath
		}
		// Check if \\?\ already exist
		if strings.HasPrefix(abspath, `\\?\`) {
			return abspath
		}
		// Check if path starts with \\
		if strings.HasPrefix(abspath, `\\`) {
			return strings.Replace(abspath, `\\`, `\\?\UNC\`, 1)
		}
		// Normal path
		return `\\?\` + abspath
	}
	return name
}

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

	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 10000; i++ {
		randSuffix := strconv.Itoa(int(1e9 + rnd.Intn(1e9)%1e9))[1:]
		path := filepath.Join(dir, prefix+randSuffix)

		ptr, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return nil, err
		}
		h, err := windows.CreateFile(ptr, access, share, nil, creation, flags, 0)
		if os.IsExist(err) {
			continue
		}
		return os.NewFile(uintptr(h), path), err
	}

	// Proper error handling is still to do
	return nil, os.ErrExist
}

// Chmod changes the mode of the named file to mode.
func Chmod(name string, mode os.FileMode) error {
	return os.Chmod(fixpath(name), mode)
}
