package fs

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/restic/restic/internal/test"
)

func verifyFileContentOpenFile(t testing.TB, fs FS, filename string, want []byte) {
	f, err := fs.OpenFile(filename, O_RDONLY, false)
	if err != nil {
		t.Fatal(err)
	}

	buf, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}

	err = f.Close()
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(want, buf) {
		t.Error(cmp.Diff(want, buf))
	}
}

func verifyDirectoryContents(t testing.TB, fs FS, dir string, want []string) {
	f, err := fs.OpenFile(dir, O_RDONLY, false)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}

	err = f.Close()
	if err != nil {
		t.Fatal(err)
	}

	sort.Strings(want)
	sort.Strings(entries)

	if !cmp.Equal(want, entries) {
		t.Error(cmp.Diff(want, entries))
	}
}

func checkFileInfo(t testing.TB, fi *ExtendedFileInfo, filename string, modtime time.Time, mode os.FileMode, isdir bool) {
	if fi.Mode.IsDir() != isdir {
		t.Errorf("IsDir returned %t, want %t", fi.Mode.IsDir(), isdir)
	}

	if fi.Mode != mode {
		t.Errorf("Mode has wrong value, want 0%o, got 0%o", mode, fi.Mode)
	}

	if !fi.ModTime.Equal(modtime) {
		t.Errorf("ModTime has wrong value, want %v, got %v", modtime, fi.ModTime)
	}

	if path.Base(fi.Name) != fi.Name {
		t.Errorf("Name is not base, want %q, got %q", path.Base(fi.Name), fi.Name)
	}

	if fi.Name != path.Base(filename) {
		t.Errorf("Name has wrong value, want %q, got %q", path.Base(filename), fi.Name)
	}
}

type fsTest []struct {
	name string
	f    func(t *testing.T, fs FS)
}

func createReadDirTest(fpath, filename string) fsTest {
	return fsTest{
		{
			name: "Readdirnames-slash-" + fpath,
			f: func(t *testing.T, fs FS) {
				verifyDirectoryContents(t, fs, "/"+fpath, []string{filename})
			},
		},
		{
			name: "Readdirnames-current-" + fpath,
			f: func(t *testing.T, fs FS) {
				verifyDirectoryContents(t, fs, path.Clean(fpath), []string{filename})
			},
		},
	}
}

func createFileTest(filename string, now time.Time, data []byte) fsTest {
	return fsTest{
		{
			name: "file/OpenFile",
			f: func(t *testing.T, fs FS) {
				verifyFileContentOpenFile(t, fs, filename, data)
			},
		},
		{
			name: "file/Lstat",
			f: func(t *testing.T, fs FS) {
				fi, err := fs.Lstat(filename)
				if err != nil {
					t.Fatal(err)
				}

				checkFileInfo(t, fi, filename, now, 0644, false)
			},
		},
		{
			name: "file/Stat",
			f: func(t *testing.T, fs FS) {
				fi := fsOpenAndStat(t, fs, filename, true)
				checkFileInfo(t, fi, filename, now, 0644, false)
			},
		},
	}
}

func createDirTest(fpath string, now time.Time) fsTest {
	return fsTest{
		{
			name: "dir/Lstat-slash-" + fpath,
			f: func(t *testing.T, fs FS) {
				fi, err := fs.Lstat("/" + fpath)
				if err != nil {
					t.Fatal(err)
				}

				checkFileInfo(t, fi, "/"+fpath, now, os.ModeDir|0755, true)
			},
		},
		{
			name: "dir/Lstat-current-" + fpath,
			f: func(t *testing.T, fs FS) {
				fi, err := fs.Lstat("./" + fpath)
				if err != nil {
					t.Fatal(err)
				}

				checkFileInfo(t, fi, "/"+fpath, now, os.ModeDir|0755, true)
			},
		},
		{
			name: "dir/Lstat-error-not-exist-" + fpath,
			f: func(t *testing.T, fs FS) {
				_, err := fs.Lstat(fpath + "/other")
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
			},
		},
		{
			name: "dir/Open-slash-" + fpath,
			f: func(t *testing.T, fs FS) {
				fi := fsOpenAndStat(t, fs, "/"+fpath, false)
				checkFileInfo(t, fi, "/"+fpath, now, os.ModeDir|0755, true)
			},
		},
		{
			name: "dir/Open-current-" + fpath,
			f: func(t *testing.T, fs FS) {
				fi := fsOpenAndStat(t, fs, "./"+fpath, false)
				checkFileInfo(t, fi, "/"+fpath, now, os.ModeDir|0755, true)
			},
		},
	}
}

func fsOpenAndStat(t *testing.T, fs FS, fpath string, metadataOnly bool) *ExtendedFileInfo {
	f, err := fs.OpenFile(fpath, O_RDONLY, metadataOnly)
	if err != nil {
		t.Fatal(err)
	}

	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	err = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	return fi
}

func TestFSReader(t *testing.T) {
	data := test.Random(55, 1<<18+588)
	now := time.Now()
	filename := "foobar"

	tests := createReadDirTest("", filename)
	tests = append(tests, createFileTest(filename, now, data)...)
	tests = append(tests, createDirTest("", now)...)

	for _, test := range tests {
		fs := NewReader(filename, io.NopCloser(bytes.NewReader(data)), ReaderOptions{
			Mode:    0644,
			Size:    int64(len(data)),
			ModTime: now,
		})

		t.Run(test.name, func(t *testing.T) {
			test.f(t, fs)
		})
	}
}

func TestFSReaderNested(t *testing.T) {
	data := test.Random(55, 1<<18+588)
	now := time.Now()
	filename := "foo/sub/bar"

	tests := createReadDirTest("", "foo")
	tests = append(tests, createReadDirTest("foo", "sub")...)
	tests = append(tests, createReadDirTest("foo/sub", "bar")...)
	tests = append(tests, createFileTest(filename, now, data)...)
	tests = append(tests, createDirTest("", now)...)
	tests = append(tests, createDirTest("foo", now)...)
	tests = append(tests, createDirTest("foo/sub", now)...)

	for _, test := range tests {
		fs := NewReader(filename, io.NopCloser(bytes.NewReader(data)), ReaderOptions{
			Mode:    0644,
			Size:    int64(len(data)),
			ModTime: now,
		})

		t.Run(test.name, func(t *testing.T) {
			test.f(t, fs)
		})
	}
}

func TestFSReaderDir(t *testing.T) {
	data := test.Random(55, 1<<18+588)
	now := time.Now()

	var tests = []struct {
		name     string
		filename string
	}{
		{
			name:     "Lstat-absolute",
			filename: "/path/to/foobar",
		},
		{
			name:     "Lstat-relative",
			filename: "path/to/foobar",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := NewReader(test.filename, io.NopCloser(bytes.NewReader(data)), ReaderOptions{
				Mode:    0644,
				Size:    int64(len(data)),
				ModTime: now,
			})

			dir := path.Dir(test.filename)
			for {
				if dir == "/" || dir == "." {
					break
				}

				fi, err := fs.Lstat(dir)
				if err != nil {
					t.Fatal(err)
				}

				checkFileInfo(t, fi, dir, now, os.ModeDir|0755, true)

				dir = path.Dir(dir)
			}
		})
	}
}

func TestFSReaderMinFileSize(t *testing.T) {
	var tests = []struct {
		name        string
		data        string
		allowEmpty  bool
		readMustErr bool
	}{
		{
			name: "regular",
			data: "foobar",
		},
		{
			name:        "empty",
			data:        "",
			allowEmpty:  false,
			readMustErr: true,
		},
		{
			name:        "empty2",
			data:        "",
			allowEmpty:  true,
			readMustErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := NewReader("testfile", io.NopCloser(strings.NewReader(test.data)), ReaderOptions{
				Mode:           0644,
				ModTime:        time.Now(),
				AllowEmptyFile: test.allowEmpty,
			})

			f, err := fs.OpenFile("testfile", O_RDONLY, false)
			if err != nil {
				t.Fatal(err)
			}

			buf, err := io.ReadAll(f)
			if test.readMustErr {
				if err == nil {
					t.Fatal("expected error not found, got nil")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
			}

			if string(buf) != test.data {
				t.Fatalf("wrong data returned, want %q, got %q", test.data, string(buf))
			}

			err = f.Close()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
