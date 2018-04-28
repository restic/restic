package fs

import (
	"bytes"
	"io/ioutil"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/restic/restic/internal/test"
)

func verifyFileContentOpen(t testing.TB, fs FS, filename string, want []byte) {
	f, err := fs.Open(filename)
	if err != nil {
		t.Fatal(err)
	}

	buf, err := ioutil.ReadAll(f)
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

func verifyFileContentOpenFile(t testing.TB, fs FS, filename string, want []byte) {
	f, err := fs.OpenFile(filename, O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}

	buf, err := ioutil.ReadAll(f)
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
	f, err := fs.Open(dir)
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

	sort.Sort(sort.StringSlice(want))
	sort.Sort(sort.StringSlice(entries))

	if !cmp.Equal(want, entries) {
		t.Error(cmp.Diff(want, entries))
	}
}

type fiSlice []os.FileInfo

func (s fiSlice) Len() int {
	return len(s)
}

func (s fiSlice) Less(i, j int) bool {
	return s[i].Name() < s[j].Name()
}

func (s fiSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func verifyDirectoryContentsFI(t testing.TB, fs FS, dir string, want []os.FileInfo) {
	f, err := fs.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := f.Readdir(-1)
	if err != nil {
		t.Fatal(err)
	}

	err = f.Close()
	if err != nil {
		t.Fatal(err)
	}

	sort.Sort(fiSlice(want))
	sort.Sort(fiSlice(entries))

	if len(want) != len(entries) {
		t.Errorf("wrong number of entries returned, want %d, got %d", len(want), len(entries))
	}
	max := len(want)
	if len(entries) < max {
		max = len(entries)
	}

	for i := 0; i < max; i++ {
		fi1 := want[i]
		fi2 := entries[i]

		if fi1.Name() != fi2.Name() {
			t.Errorf("entry %d: wrong value for Name: want %q, got %q", i, fi1.Name(), fi2.Name())
		}

		if fi1.IsDir() != fi2.IsDir() {
			t.Errorf("entry %d: wrong value for IsDir: want %v, got %v", i, fi1.IsDir(), fi2.IsDir())
		}

		if fi1.Mode() != fi2.Mode() {
			t.Errorf("entry %d: wrong value for Mode: want %v, got %v", i, fi1.Mode(), fi2.Mode())
		}

		if fi1.ModTime() != fi2.ModTime() {
			t.Errorf("entry %d: wrong value for ModTime: want %v, got %v", i, fi1.ModTime(), fi2.ModTime())
		}

		if fi1.Size() != fi2.Size() {
			t.Errorf("entry %d: wrong value for Size: want %v, got %v", i, fi1.Size(), fi2.Size())
		}

		if fi1.Sys() != fi2.Sys() {
			t.Errorf("entry %d: wrong value for Sys: want %v, got %v", i, fi1.Sys(), fi2.Sys())
		}
	}
}

func checkFileInfo(t testing.TB, fi os.FileInfo, filename string, modtime time.Time, mode os.FileMode, isdir bool) {
	if fi.IsDir() {
		t.Errorf("IsDir returned true, want false")
	}

	if fi.Mode() != mode {
		t.Errorf("Mode() returned wrong value, want 0%o, got 0%o", mode, fi.Mode())
	}

	if !modtime.Equal(time.Time{}) && !fi.ModTime().Equal(modtime) {
		t.Errorf("ModTime() returned wrong value, want %v, got %v", modtime, fi.ModTime())
	}

	if fi.Name() != filename {
		t.Errorf("Name() returned wrong value, want %q, got %q", filename, fi.Name())
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
			name: "Readdir-slash",
			f: func(t *testing.T, fs FS) {
				fi := fakeFileInfo{
					mode:    0644,
					modtime: now,
					name:    filename,
					size:    int64(len(data)),
				}
				verifyDirectoryContentsFI(t, fs, "/", []os.FileInfo{fi})
			},
		},
		{
			name: "Readdir-current",
			f: func(t *testing.T, fs FS) {
				fi := fakeFileInfo{
					mode:    0644,
					modtime: now,
					name:    filename,
					size:    int64(len(data)),
				}
				verifyDirectoryContentsFI(t, fs, ".", []os.FileInfo{fi})
			},
		},
		{
			name: "file/Open",
			f: func(t *testing.T, fs FS) {
				verifyFileContentOpen(t, fs, filename, data)
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
				f, err := fs.Open(filename)
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

				checkFileInfo(t, fi, "/", time.Time{}, 0755, false)
			},
		},
		{
			name: "dir/Lstat-current",
			f: func(t *testing.T, fs FS) {
				fi, err := fs.Lstat(".")
				if err != nil {
					t.Fatal(err)
				}

				checkFileInfo(t, fi, ".", time.Time{}, 0755, false)
			},
		},
		{
			name: "dir/Open-slash",
			f: func(t *testing.T, fs FS) {
				fi, err := fs.Lstat("/")
				if err != nil {
					t.Fatal(err)
				}

				checkFileInfo(t, fi, "/", time.Time{}, 0755, false)
			},
		},
		{
			name: "dir/Open-current",
			f: func(t *testing.T, fs FS) {
				fi, err := fs.Lstat(".")
				if err != nil {
					t.Fatal(err)
				}

				checkFileInfo(t, fi, ".", time.Time{}, 0755, false)
			},
		},
	}

	for _, test := range tests {
		fs := &Reader{
			Name:       filename,
			ReadCloser: ioutil.NopCloser(bytes.NewReader(data)),

			Mode:    0644,
			Size:    int64(len(data)),
			ModTime: now,
		}

		t.Run(test.name, func(t *testing.T) {
			test.f(t, fs)
		})
	}
}
