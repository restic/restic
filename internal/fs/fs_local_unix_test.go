//go:build unix

package fs

import (
	"syscall"
	"testing"

	"github.com/restic/restic/internal/data"
	rtest "github.com/restic/restic/internal/test"
)

func TestFSLocalMetadataUnix(t *testing.T) {
	for _, test := range []fsLocalMetadataTestcase{
		{
			name: "socket",
			setup: func(t *testing.T, path string) {
				fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
				rtest.OK(t, err)
				defer func() {
					_ = syscall.Close(fd)
				}()

				addr := &syscall.SockaddrUnix{Name: path}
				rtest.OK(t, syscall.Bind(fd, addr))
			},
			nodeType: data.NodeTypeSocket,
		},
		{
			name: "fifo",
			setup: func(t *testing.T, path string) {
				rtest.OK(t, mkfifo(path, 0o600))
			},
			nodeType: data.NodeTypeFifo,
		},
		// device files can only be created as root
	} {
		runFSLocalTestcase(t, test)
	}
}
