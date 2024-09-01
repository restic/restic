//go:build windows
// +build windows

package fs

import (
	"os"

	"github.com/restic/restic/internal/errors"
)

// deviceID extracts the device ID from an os.FileInfo object by casting it
// to syscall.Stat_t
func deviceID(_ os.FileInfo) (deviceID uint64, err error) {
	return 0, errors.New("Device IDs are not supported on Windows")
}
