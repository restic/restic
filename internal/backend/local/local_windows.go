package local

import (
	"os"
)

// We don't modify read-only on windows,
// since it will make us unable to delete the file,
// and this isn't common practice on this platform.
func setFileReadonly(f string, mode os.FileMode) error {
	return nil
}
