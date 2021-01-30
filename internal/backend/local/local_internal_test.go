package local

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"

	"github.com/cenkalti/backoff/v4"
)

func TestNoSpacePermanent(t *testing.T) {
	oldOpenFile := openFile
	defer func() {
		openFile = oldOpenFile
	}()

	openFile = func(name string, flags int, mode os.FileMode) (*os.File, error) {
		// The actual error from os.OpenFile is *os.PathError.
		// Other functions called inside Save may return *os.SyscallError.
		return nil, os.NewSyscallError("open", syscall.ENOSPC)
	}

	dir, cleanup := rtest.TempDir(t)
	defer cleanup()

	be, err := Open(context.Background(), Config{Path: dir})
	rtest.OK(t, err)
	defer func() {
		rtest.OK(t, be.Close())
	}()

	h := restic.Handle{Type: restic.ConfigFile}
	err = be.Save(context.Background(), h, nil)
	_, ok := err.(*backoff.PermanentError)
	rtest.Assert(t, ok,
		"error type should be backoff.PermanentError, got %T", err)
	rtest.Assert(t, errors.Is(err, syscall.ENOSPC),
		"could not recover original ENOSPC error")
}
