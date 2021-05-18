package termstatus

import (
	"syscall"
	"testing"

	"golang.org/x/sys/windows"

	rtest "github.com/restic/restic/internal/test"
)

func TestIsMinTTY(t *testing.T) {
	for _, test := range []struct {
		path   string
		result bool
	}{
		{`\\.\pipe\msys-dd50a72ab4668b33-pty0-to-master`, true},
		{`\\.\pipe\msys-dd50a72ab4668b33-13244-pipe-0x16`, false},
	} {
		filename, err := syscall.UTF16FromString(test.path)
		rtest.OK(t, err)
		handle, err := windows.CreateNamedPipe(&filename[0], windows.PIPE_ACCESS_DUPLEX,
			windows.PIPE_TYPE_BYTE, 1, 1024, 1024, 0, nil)
		rtest.OK(t, err)
		defer windows.CloseHandle(handle)

		rtest.Assert(t, CanUpdateStatus(uintptr(handle)) == test.result,
			"expected CanUpdateStatus(%v) == %v", test.path, test.result)
	}
}
