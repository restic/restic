package restorer

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/restic/restic/internal/errors"
	rtest "github.com/restic/restic/internal/test"
)

func TestFilesWriterBasic(t *testing.T) {
	dir := rtest.TempDir(t)
	w := newFilesWriter(1)

	f1 := dir + "/f1"
	f2 := dir + "/f2"

	rtest.OK(t, w.writeToFile(f1, []byte{1}, 0, 2, false))
	rtest.Equals(t, 0, len(w.buckets[0].files))

	rtest.OK(t, w.writeToFile(f2, []byte{2}, 0, 2, false))
	rtest.Equals(t, 0, len(w.buckets[0].files))

	rtest.OK(t, w.writeToFile(f1, []byte{1}, 1, -1, false))
	rtest.Equals(t, 0, len(w.buckets[0].files))

	rtest.OK(t, w.writeToFile(f2, []byte{2}, 1, -1, false))
	rtest.Equals(t, 0, len(w.buckets[0].files))

	buf, err := os.ReadFile(f1)
	rtest.OK(t, err)
	rtest.Equals(t, []byte{1, 1}, buf)

	buf, err = os.ReadFile(f2)
	rtest.OK(t, err)
	rtest.Equals(t, []byte{2, 2}, buf)
}

func TestCreateFile(t *testing.T) {
	basepath := filepath.Join(t.TempDir(), "test")

	scenarios := []struct {
		name   string
		create func(t testing.TB, path string)
		err    error
	}{
		{
			"file",
			func(t testing.TB, path string) {
				rtest.OK(t, os.WriteFile(path, []byte("test-test-test-data"), 0o400))
			},
			nil,
		},
		{
			"empty dir",
			func(t testing.TB, path string) {
				rtest.OK(t, os.Mkdir(path, 0o400))
			},
			nil,
		},
		{
			"symlink",
			func(t testing.TB, path string) {
				rtest.OK(t, os.Symlink("./something", path))
			},
			nil,
		},
		{
			"filled dir",
			func(t testing.TB, path string) {
				rtest.OK(t, os.Mkdir(path, 0o700))
				rtest.OK(t, os.WriteFile(filepath.Join(path, "file"), []byte("data"), 0o400))
			},
			syscall.ENOTEMPTY,
		},
	}

	tests := []struct {
		size     int64
		isSparse bool
	}{
		{5, false},
		{21, false},
		{100, false},
		{5, true},
		{21, true},
		{100, true},
	}

	for i, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			for _, test := range tests {
				path := basepath + fmt.Sprintf("%v", i)
				sc.create(t, path)
				f, err := createFile(path, test.size, test.isSparse)
				if sc.err == nil {
					rtest.OK(t, err)
					fi, err := f.Stat()
					rtest.OK(t, err)
					rtest.Assert(t, fi.Mode().IsRegular(), "wrong filetype %v", fi.Mode())
					rtest.Assert(t, fi.Size() <= test.size, "unexpected file size expected %v, got %v", test.size, fi.Size())
					rtest.OK(t, f.Close())
				} else {
					rtest.Assert(t, errors.Is(err, sc.err), "unexpected error got %v expected %v", err, sc.err)
				}
				rtest.OK(t, os.RemoveAll(path))
			}
		})
	}
}
