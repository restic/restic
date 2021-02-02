package fs

import (
	"os"

	"golang.org/x/sys/unix"
)

func PreallocateFile(wr *os.File, size int64) error {
	if size <= 0 {
		return nil
	}
	// int fallocate(int fd, int mode, off_t offset, off_t len)
	// use mode = 0 to also change the file size
	return unix.Fallocate(int(wr.Fd()), 0, 0, size)
}
