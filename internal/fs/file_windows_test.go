package fs_test

import (
	"errors"
	"os"
	"testing"

	"github.com/restic/restic/internal/fs"
	rtest "github.com/restic/restic/internal/test"
)

func TestTempFile(t *testing.T) {
	// create two temp files at the same time to check that the
	// collision avoidance works
	f, err := fs.TempFile("", "test")
	fn := f.Name()
	rtest.OK(t, err)
	f2, err := fs.TempFile("", "test")
	fn2 := f2.Name()
	rtest.OK(t, err)
	rtest.Assert(t, fn != fn2, "filenames don't differ %s", fn)

	_, err = os.Stat(fn)
	rtest.OK(t, err)
	_, err = os.Stat(fn2)
	rtest.OK(t, err)

	rtest.OK(t, f.Close())
	rtest.OK(t, f2.Close())

	_, err = os.Stat(fn)
	rtest.Assert(t, errors.Is(err, os.ErrNotExist), "err %s", err)
	_, err = os.Stat(fn2)
	rtest.Assert(t, errors.Is(err, os.ErrNotExist), "err %s", err)
}
