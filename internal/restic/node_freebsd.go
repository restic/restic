//go:build freebsd
// +build freebsd

package restic

import (
	"os"
	"syscall"
)

func (node Node) restoreSymlinkTimestamps(path string, utimes [2]syscall.Timespec) error {
	return nil
}

func mknod(path string, mode uint32, dev uint64) error {
	err := syscall.Mknod(path, mode, dev)
	if err != nil {
		err = &os.PathError{Op: "mknod", Path: path, Err: err}
	}
	return err
}

func (s statT) atim() syscall.Timespec { return s.Atimespec }
func (s statT) mtim() syscall.Timespec { return s.Mtimespec }
func (s statT) ctim() syscall.Timespec { return s.Ctimespec }
