package local

import (
	"os"
)

// Can't explicitly flush directory changes on Windows.
func fsyncDir(_ string) error { return nil }

// Windows is not macOS.
func isMacENOTTY(_ error) bool { return false }

// We don't modify read-only on windows,
// since it will make us unable to delete the file,
// and this isn't common practice on this platform.
func setFileReadonly(_ string, _ os.FileMode) error {
	return nil
}
