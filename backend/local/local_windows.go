package local

import (
	"os"
)

// We don't modify read-only on windows,
// since it will make us unable to delete the file,
// and this isn't common practice on this platform.
func setNewFileMode(f string, fi os.FileInfo) error {
	return nil
}
