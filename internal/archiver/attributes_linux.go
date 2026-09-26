//go:build linux

package archiver

import (
	"golang.org/x/sys/unix"
)

// isNoDump returns whether the "no dump" Linux file attribute is set on path.
// See CHATTR(1) for more information about Linux file attributes.
func isNoDump(path string) (bool, error) {
	statx := &unix.Statx_t{}

	if err := unix.Statx(0, path, unix.AT_NO_AUTOMOUNT|unix.AT_SYMLINK_NOFOLLOW, 0, statx); err != nil {
		return false, err
	}

	return statx.Attributes&unix.STATX_ATTR_NODUMP != 0, nil
}
