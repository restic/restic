package restorer

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/restic/restic/internal/errors"
	rtest "github.com/restic/restic/internal/test"
)

func TestFilesWriterBasic(t *testing.T) {
	dir := rtest.TempDir(t)
	w := newFilesWriter(1, false)

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

	rtest.OK(t, w.flush())

	buf, err := os.ReadFile(f1)
	rtest.OK(t, err)
	rtest.Equals(t, []byte{1, 1}, buf)

	buf, err = os.ReadFile(f2)
	rtest.OK(t, err)
	rtest.Equals(t, []byte{2, 2}, buf)
}

// A close that fails leaves a file that may not have reached the disk, so
// flush reports it instead of dropping it. 0.19.0 moved the handles into the
// cache, whose eviction callback closed them and threw the error away
// (#22179).
func TestFilesWriterFlushReportsCloseError(t *testing.T) {
	path := filepath.Join(rtest.TempDir(t), "f")
	rtest.OK(t, os.WriteFile(path, []byte{0, 0}, 0o600))
	w := newFilesWriter(1, false)
	rtest.OK(t, w.writeToFile(path, []byte{1}, 0, -1, false))

	// close the handle the cache holds behind the writer's back: the close
	// the cache then makes fails, as one on a full disk does
	cached, ok := w.cache.Get(path)
	rtest.Assert(t, ok, "the file's handle isn't cached")
	rtest.OK(t, cached.File.Close())

	err := w.flush()
	rtest.Assert(t, errors.Is(err, os.ErrClosed), "flush reported %v, want the close error", err)
}

func TestFilesWriterRecursiveOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test")

	// create filled directory
	rtest.OK(t, os.Mkdir(path, 0o700))
	rtest.OK(t, os.WriteFile(filepath.Join(path, "file"), []byte("data"), 0o400))

	// must error if recursive delete is not allowed
	w := newFilesWriter(1, false)
	err := w.writeToFile(path, []byte{1}, 0, 2, false)
	rtest.Assert(t, errors.Is(err, notEmptyDirError()), "unexpected error got %v", err)
	rtest.Equals(t, 0, len(w.buckets[0].files))
	rtest.OK(t, w.flush())

	// must replace directory
	w = newFilesWriter(1, true)
	rtest.OK(t, w.writeToFile(path, []byte{1, 1}, 0, 2, false))
	rtest.Equals(t, 0, len(w.buckets[0].files))
	rtest.OK(t, w.flush())

	buf, err := os.ReadFile(path)
	rtest.OK(t, err)
	rtest.Equals(t, []byte{1, 1}, buf)
}

func TestCreateFile(t *testing.T) {
	basepath := filepath.Join(t.TempDir(), "test")

	scenarios := []struct {
		name   string
		create func(t testing.TB, path string)
		check  func(t testing.TB, path string)
		err    error
	}{
		{
			name: "file",
			create: func(t testing.TB, path string) {
				rtest.OK(t, os.WriteFile(path, []byte("test-test-test-data"), 0o400))
			},
		},
		{
			name: "empty dir",
			create: func(t testing.TB, path string) {
				rtest.OK(t, os.Mkdir(path, 0o400))
			},
		},
		{
			name: "symlink",
			create: func(t testing.TB, path string) {
				rtest.OK(t, os.Symlink("./something", path))
			},
		},
		{
			name: "filled dir",
			create: func(t testing.TB, path string) {
				rtest.OK(t, os.Mkdir(path, 0o700))
				rtest.OK(t, os.WriteFile(filepath.Join(path, "file"), []byte("data"), 0o400))
			},
			err: notEmptyDirError(),
		},
		{
			name: "hardlinks",
			create: func(t testing.TB, path string) {
				rtest.OK(t, os.WriteFile(path, []byte("test-test-test-data"), 0o400))
				rtest.OK(t, os.Link(path, path+"h"))
			},
			check: func(t testing.TB, path string) {
				if runtime.GOOS == "windows" {
					// hardlinks are not supported on windows
					return
				}

				data, err := os.ReadFile(path + "h")
				rtest.OK(t, err)
				rtest.Equals(t, "test-test-test-data", string(data), "unexpected content change")
			},
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
			for j, test := range tests {
				path := basepath + fmt.Sprintf("%v%v", i, j)
				sc.create(t, path)
				f, err := createFile(path, test.size, test.isSparse, false)
				if sc.err == nil {
					rtest.OK(t, err)
					fi, err := f.Stat()
					rtest.OK(t, err)
					rtest.Assert(t, fi.Mode().IsRegular(), "wrong filetype %v", fi.Mode())
					rtest.Assert(t, fi.Size() <= test.size, "unexpected file size expected %v, got %v", test.size, fi.Size())
					rtest.OK(t, f.Close())
					if sc.check != nil {
						sc.check(t, path)
					}
				} else {
					rtest.Assert(t, errors.Is(err, sc.err), "unexpected error got %v expected %v", err, sc.err)
				}
				rtest.OK(t, os.RemoveAll(path))
			}
		})
	}
}

func TestCreateFileRecursiveDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test")

	// create filled directory
	rtest.OK(t, os.Mkdir(path, 0o700))
	rtest.OK(t, os.WriteFile(filepath.Join(path, "file"), []byte("data"), 0o400))

	// replace it
	f, err := createFile(path, 42, false, true)
	rtest.OK(t, err)
	fi, err := f.Stat()
	rtest.OK(t, err)
	rtest.Assert(t, fi.Mode().IsRegular(), "wrong filetype %v", fi.Mode())
	rtest.OK(t, f.Close())
}
