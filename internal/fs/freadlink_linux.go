package fs

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// based on readlink from go/src/os/file_unix.go Go 1.23.2
// modified to use Readlinkat syscall instead of readlink

// Many functions in package syscall return a count of -1 instead of 0.
// Using fixCount(call()) instead of call() corrects the count.
func fixCount(n int, err error) (int, error) {
	if n < 0 {
		n = 0
	}
	return n, err
}

func Freadlink(fd uintptr, name string) (string, error) {
	for namelen := 128; ; namelen *= 2 {
		b := make([]byte, namelen)
		var (
			n int
			e error
		)
		for {
			n, e = fixCount(freadlink(int(fd), b))
			if e != syscall.EINTR {
				break
			}
		}
		if e != nil {
			return "", &os.PathError{Op: "readlink", Path: name, Err: e}
		}
		if n < namelen {
			return string(b[0:n]), nil
		}
	}
}

func freadlink(fd int, buf []byte) (n int, err error) {
	// pass empty path to process the link itself
	return unix.Readlinkat(fd, "", buf)
}
