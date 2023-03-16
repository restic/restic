package local

import (
	"os"
)

// Can't explicitly flush directory changes on Windows.
func fsyncDir(dir string) error { return nil }

// Windows is not macOS.
func isMacENOTTY(err error) bool { return false }

// We don't modify read-only on windows,
// since it will make us unable to delete the file,
// and this isn't common practice on this platform.
func setFileReadonly(f string, mode os.FileMode) error {
	return nil
}
