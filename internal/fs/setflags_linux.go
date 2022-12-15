package fs

import (
	"os"

	"golang.org/x/sys/unix"
)

// SetFlags tries to set the O_NOATIME flag on f, which prevents the kernel
// from updating the atime on a read call.
//
// The call fails when we're not the owner of the file or root. The caller
// should ignore the error, which is returned for testing only.
func setFlags(f *os.File) error {
	fd := f.Fd()
	flags, err := unix.FcntlInt(fd, unix.F_GETFL, 0)
	if err == nil {
		_, err = unix.FcntlInt(fd, unix.F_SETFL, flags|unix.O_NOATIME)
	}
	return err
}
