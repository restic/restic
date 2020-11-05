// +build !windows

package fs

import (
	"os"
	"syscall"
	"testing"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/test"
)

func TestChmodEINTR(t *testing.T) {
	defer func() { osChmod = os.Chmod }()

	numFail := 10
	osChmod = func(filename string, mode os.FileMode) error {
		numFail--
		if numFail >= 0 {
			return &os.PathError{Err: syscall.EINTR}
		}
		return os.Chmod(filename, mode)
	}

	err := Chmod("/no/file/here", 0700)
	test.Assert(t, os.IsNotExist(err), "wrong error from Chmod: %v", err)

	numFail = 9999
	err = Chmod("/no/file/here", 0700)
	err = errors.Cause(err)
	switch err := err.(type) {
	case *os.PathError:
		test.Equals(t, syscall.EINTR, err.Err)
	default:
		t.Error(err)
	}
}

func TestChmodENOTSUP(t *testing.T) {
	defer func() { osChmod = os.Chmod }()

	osChmod = func(string, os.FileMode) error {
		return &os.PathError{Err: syscall.ENOTSUP}
	}

	test.OK(t, Chmod("/whatever", 0600))
}
