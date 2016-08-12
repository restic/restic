// +build !linux !go1.4

package fs

import (
	"os"
	"restic/patched/os"
)


// Open opens a file for reading.
func Open(name string) (File, error) {
	return patchedos.OpenFile(name, os.O_RDONLY, 0)
}

// ClearCache syncs and then removes the file's content from the OS cache.
func ClearCache(f File) error {
	return nil
}
