// +build !windows

package fs

import (
	"os"
	"syscall"

	"restic/errors"
)

// DeviceID extracts the device ID from an os.FileInfo object by casting it
// to syscall.Stat_t
func DeviceID(fi os.FileInfo) (deviceID uint64, err error) {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		// st.Dev is uint32 on Darwin and uint64 on Linux. Just cast
		// everything to uint64.
		return uint64(st.Dev), nil
	}
	return 0, errors.New("Could not cast to syscall.Stat_t")
}
