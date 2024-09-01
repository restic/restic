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
	f, err := fs.OpenFile(filename, O_RDONLY, 0)
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
	f, err := fs.OpenFile(dir, os.O_RDONLY, 0)
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

func checkFileInfo(t testing.TB, fi os.FileInfo, filename string, modtime time.Time, mode os.FileMode, isdir bool) {
	if fi.IsDir() != isdir {
		t.Errorf("IsDir returned %t, want %t", fi.IsDir(), isdir)
	}

	if fi.Mode() != mode {
		t.Errorf("Mode() returned wrong value, want 0%o, got 0%o", mode, fi.Mode())
	}

	if !modtime.Equal(time.Time{}) && !fi.ModTime().Equal(modtime) {
		t.Errorf("ModTime() returned wrong value, want %v, got %v", modtime, fi.ModTime())
	}

	if path.Base(fi.Name()) != fi.Name() {
		t.Errorf("Name() returned is not base, want %q, got %q", path.Base(fi.Name()), fi.Name())
	}

	if fi.Name() != path.Base(filename) {
		t.Errorf("Name() returned wrong value, want %q, got %q", path.Base(filename), fi.Name())
	}
}

func TestFSReader(t *testing.T) {
	data := test.Random(55, 1<<18+588)
	now := time.Now()
	filename := "foobar"

	var tests = []struct {
		name string
		f    func(t *testing.T, fs FS)
	}{
		{
			name: "Readdirnames-slash",
			f: func(t *testing.T, fs FS) {
				verifyDirectoryContents(t, fs, "/", []string{filename})
			},
		},
		{
			name: "Readdirnames-current",
			f: func(t *testing.T, fs FS) {
				verifyDirectoryContents(t, fs, ".", []string{filename})
			},
		},
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
				f, err := fs.OpenFile(filename, os.O_RDONLY, 0)
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

				checkFileInfo(t, fi, filename, now, 0644, false)
			},
		},
		{
			name: "dir/Lstat-slash",
			f: func(t *testing.T, fs FS) {
				fi, err := fs.Lstat("/")
				if err != nil {
					t.Fatal(err)
				}

				checkFileInfo(t, fi, "/", time.Time{}, os.ModeDir|0755, true)
			},
		},
		{
			name: "dir/Lstat-current",
			f: func(t *testing.T, fs FS) {
				fi, err := fs.Lstat(".")
				if err != nil {
					t.Fatal(err)
				}

				checkFileInfo(t, fi, ".", time.Time{}, os.ModeDir|0755, true)
			},
		},
		{
			name: "dir/Lstat-error-not-exist",
			f: func(t *testing.T, fs FS) {
				_, err := fs.Lstat("other")
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
			},
		},
		{
			name: "dir/Open-slash",
			f: func(t *testing.T, fs FS) {
				fi, err := fs.Lstat("/")
				if err != nil {
					t.Fatal(err)
				}

				checkFileInfo(t, fi, "/", time.Time{}, os.ModeDir|0755, true)
			},
		},
		{
			name: "dir/Open-current",
			f: func(t *testing.T, fs FS) {
				fi, err := fs.Lstat(".")
				if err != nil {
					t.Fatal(err)
				}

				checkFileInfo(t, fi, ".", time.Time{}, os.ModeDir|0755, true)
			},
		},
	}

	for _, test := range tests {
		fs := &Reader{
			Name:       filename,
			ReadCloser: io.NopCloser(bytes.NewReader(data)),

			Mode:    0644,
			Size:    int64(len(data)),
			ModTime: now,
		}

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
			fs := &Reader{
				Name:       test.filename,
				ReadCloser: io.NopCloser(bytes.NewReader(data)),

				Mode:    0644,
				Size:    int64(len(data)),
				ModTime: now,
			}

			dir := path.Dir(fs.Name)
			for {
				if dir == "/" || dir == "." {
					break
				}

				fi, err := fs.Lstat(dir)
				if err != nil {
					t.Fatal(err)
				}

				checkFileInfo(t, fi, dir, time.Time{}, os.ModeDir|0755, true)

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
			fs := &Reader{
				Name:           "testfile",
				ReadCloser:     io.NopCloser(strings.NewReader(test.data)),
				Mode:           0644,
				ModTime:        time.Now(),
				AllowEmptyFile: test.allowEmpty,
			}

			f, err := fs.OpenFile("testfile", os.O_RDONLY, 0)
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
