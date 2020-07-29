// +build freebsd

package restic

import "syscall"

func (node Node) restoreSymlinkTimestamps(path string, utimes [2]syscall.Timespec) error {
	return nil
}

func (node Node) device() uint64 {
	return node.Device
}

func (s statUnix) atim() syscall.Timespec { return s.Atimespec }
func (s statUnix) mtim() syscall.Timespec { return s.Mtimespec }
func (s statUnix) ctim() syscall.Timespec { return s.Ctimespec }
