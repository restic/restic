//go:build unix

package fileio

import (
	"os"
	"syscall"
)

// deviceID returns the ID of the device holding path, which identifies the filesystem
// it lives on.
func deviceID(path string) (uint64, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true
}
