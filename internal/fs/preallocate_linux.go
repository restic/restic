package fs

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func PreallocateFile(wr *os.File, size int64) error {
	if size <= 0 {
		return nil
	}
	// int fallocate(int fd, int mode, off_t offset, off_t len)
	// use mode = 0 to also change the file size
	return ignoringEINTR(func() error { return unix.Fallocate(int(wr.Fd()), 0, 0, size) })
}

// ignoringEINTR makes a function call and repeats it if it returns
// an EINTR error.
// copied from /usr/lib/go/src/internal/poll/fd_posix.go of go 1.23.1
func ignoringEINTR(fn func() error) error {
	for {
		err := fn()
		if err != syscall.EINTR {
			return err
		}
	}
}
