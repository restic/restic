package fs

import "syscall"

func nodeRestoreSymlinkTimestamps(path string, utimes [2]syscall.Timespec) error {
	return nil
}
