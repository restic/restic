//go:build !linux
// +build !linux

package fs

import "os"

// OS-specific replacements of setFlags can set file status flags
// that improve I/O performance.
func setFlags(*os.File) error {
	return nil
}
