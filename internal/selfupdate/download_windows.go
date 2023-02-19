//go:build windows
// +build windows

package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/restic/restic/internal/errors"
)

// Rename (rather than remove) the running version. The running binary will be locked
// on Windows and cannot be removed while still executing.
func removeResticBinary(dir, target string) error {
	// nothing to do if the target does not exist
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	backup := filepath.Join(dir, filepath.Base(target)+".bak")
	if _, err := os.Stat(backup); err == nil {
		_ = os.Remove(backup)
	}
	if err := os.Rename(target, backup); err != nil {
		return fmt.Errorf("unable to rename target file: %v", err)
	}
	return nil
}
