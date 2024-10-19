//go:build !linux && unix

package fs

import (
	"syscall"

	"github.com/restic/restic/internal/restic"
)

// utimesNano is like syscall.UtimesNano, except that it skips symlinks.
func utimesNano(path string, atime, mtime int64, typ restic.NodeType) error {
	if typ == restic.NodeTypeSymlink {
		return nil
	}

	return syscall.UtimesNano(path, []syscall.Timespec{
		syscall.NsecToTimespec(atime),
		syscall.NsecToTimespec(mtime),
	})
}
