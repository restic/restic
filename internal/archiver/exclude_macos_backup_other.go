//go:build !darwin

package archiver

import "github.com/restic/restic/internal/errors"

// RejectMacOSBackupExcludes is only available on macOS.
func RejectMacOSBackupExcludes(_ func(msg string, args ...interface{})) (RejectFunc, error) {
	return nil, errors.New("macOS backup exclusions are only supported on macOS")
}
