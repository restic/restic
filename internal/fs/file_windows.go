package fs

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"strconv"
	"sync"
	"time"
)

// Random number state.
// We generate random temporary file names so that there's a good
// chance the file doesn't exist yet - keeps the number of tries in
// TempFile to a minimum.
var rand uint32
var randmu sync.Mutex

func reseed() uint32 {
	return uint32(time.Now().UnixNano() + int64(os.Getpid()))
}

func nextRandom() string {
	randmu.Lock()
	r := rand
	if r == 0 {
		r = reseed()
	}
	r = r*1664525 + 1013904223 // constants from Numerical Recipes
	rand = r
	randmu.Unlock()
	return strconv.Itoa(int(1e9 + r%1e9))[1:]
}

const (
	FILE_ATTRIBUTE_TEMPORARY  = 0x00000100
	FILE_FLAG_DELETE_ON_CLOSE = 0x04000000
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

// TempFile creates a temporary file.
func TempFile(dir, prefix string) (f *os.File, err error) {
	if dir == "" {
		dir = os.TempDir()
	}

	access := uint32(syscall.GENERIC_READ | syscall.GENERIC_WRITE)
	creation := uint32(syscall.CREATE_NEW)
	flags := uint32(FILE_ATTRIBUTE_TEMPORARY | FILE_FLAG_DELETE_ON_CLOSE)
	
	for i := 0; i < 10000; i++ {
		path := filepath.Join(dir, prefix+nextRandom())

		h, err := syscall.CreateFile(syscall.StringToUTF16Ptr(path), access, 0, nil, creation, flags, 0)
		if err == nil {
			return os.NewFile(uintptr(h), path), nil
		}
	}
	
	// Proper error handling is still to do
	return nil, os.ErrExist
}

// Chmod changes the mode of the named file to mode.
func Chmod(name string, mode os.FileMode) error {
	return os.Chmod(fixpath(name), mode)
}
