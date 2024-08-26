package layout

import (
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/restic/restic/internal/backend"
	rtest "github.com/restic/restic/internal/test"
)

func TestDefaultLayout(t *testing.T) {
	tempdir := rtest.TempDir(t)

	var tests = []struct {
		path string
		join func(...string) string
		backend.Handle
		filename string
	}{
		{
			tempdir,
			filepath.Join,
			backend.Handle{Type: backend.PackFile, Name: "0123456"},
			filepath.Join(tempdir, "data", "01", "0123456"),
		},
		{
			tempdir,
			filepath.Join,
			backend.Handle{Type: backend.ConfigFile, Name: "CFG"},
			filepath.Join(tempdir, "config"),
		},
		{
			tempdir,
			filepath.Join,
			backend.Handle{Type: backend.SnapshotFile, Name: "123456"},
			filepath.Join(tempdir, "snapshots", "123456"),
		},
		{
			tempdir,
			filepath.Join,
			backend.Handle{Type: backend.IndexFile, Name: "123456"},
			filepath.Join(tempdir, "index", "123456"),
		},
		{
			tempdir,
			filepath.Join,
			backend.Handle{Type: backend.LockFile, Name: "123456"},
			filepath.Join(tempdir, "locks", "123456"),
		},
		{
			tempdir,
			filepath.Join,
			backend.Handle{Type: backend.KeyFile, Name: "123456"},
			filepath.Join(tempdir, "keys", "123456"),
		},
		{
			"",
			path.Join,
			backend.Handle{Type: backend.PackFile, Name: "0123456"},
			"data/01/0123456",
		},
		{
			"",
			path.Join,
			backend.Handle{Type: backend.ConfigFile, Name: "CFG"},
			"config",
		},
		{
			"",
			path.Join,
			backend.Handle{Type: backend.SnapshotFile, Name: "123456"},
			"snapshots/123456",
		},
		{
			"",
			path.Join,
			backend.Handle{Type: backend.IndexFile, Name: "123456"},
			"index/123456",
		},
		{
			"",
			path.Join,
			backend.Handle{Type: backend.LockFile, Name: "123456"},
			"locks/123456",
		},
		{
			"",
			path.Join,
			backend.Handle{Type: backend.KeyFile, Name: "123456"},
			"keys/123456",
		},
	}

	t.Run("Paths", func(t *testing.T) {
		l := &DefaultLayout{
			path: tempdir,
			join: filepath.Join,
		}

		dirs := l.Paths()

		want := []string{
			filepath.Join(tempdir, "data"),
			filepath.Join(tempdir, "snapshots"),
			filepath.Join(tempdir, "index"),
			filepath.Join(tempdir, "locks"),
			filepath.Join(tempdir, "keys"),
		}

		for i := 0; i < 256; i++ {
			want = append(want, filepath.Join(tempdir, "data", fmt.Sprintf("%02x", i)))
		}

		sort.Strings(want)
		sort.Strings(dirs)

		if !reflect.DeepEqual(dirs, want) {
			t.Fatalf("wrong paths returned, want:\n  %v\ngot:\n  %v", want, dirs)
		}
	})

	for _, test := range tests {
		t.Run(fmt.Sprintf("%v/%v", test.Type, test.Handle.Name), func(t *testing.T) {
			l := &DefaultLayout{
				path: test.path,
				join: test.join,
			}

			filename := l.Filename(test.Handle)
			if filename != test.filename {
				t.Fatalf("wrong filename, want %v, got %v", test.filename, filename)
			}
		})
	}
}

func TestRESTLayout(t *testing.T) {
	url := `https://hostname.foo`

	var tests = []struct {
		backend.Handle
		filename string
	}{
		{
			backend.Handle{Type: backend.PackFile, Name: "0123456"},
			strings.Join([]string{url, "data", "0123456"}, "/"),
		},
		{
			backend.Handle{Type: backend.ConfigFile, Name: "CFG"},
			strings.Join([]string{url, "config"}, "/"),
		},
		{
			backend.Handle{Type: backend.SnapshotFile, Name: "123456"},
			strings.Join([]string{url, "snapshots", "123456"}, "/"),
		},
		{
			backend.Handle{Type: backend.IndexFile, Name: "123456"},
			strings.Join([]string{url, "index", "123456"}, "/"),
		},
		{
			backend.Handle{Type: backend.LockFile, Name: "123456"},
			strings.Join([]string{url, "locks", "123456"}, "/"),
		},
		{
			backend.Handle{Type: backend.KeyFile, Name: "123456"},
			strings.Join([]string{url, "keys", "123456"}, "/"),
		},
	}

	l := &RESTLayout{
		url: url,
	}

	t.Run("Paths", func(t *testing.T) {
		dirs := l.Paths()

		want := []string{
			strings.Join([]string{url, "data"}, "/"),
			strings.Join([]string{url, "snapshots"}, "/"),
			strings.Join([]string{url, "index"}, "/"),
			strings.Join([]string{url, "locks"}, "/"),
			strings.Join([]string{url, "keys"}, "/"),
		}

		sort.Strings(want)
		sort.Strings(dirs)

		if !reflect.DeepEqual(dirs, want) {
			t.Fatalf("wrong paths returned, want:\n  %v\ngot:\n  %v", want, dirs)
		}
	})

	for _, test := range tests {
		t.Run(fmt.Sprintf("%v/%v", test.Type, test.Handle.Name), func(t *testing.T) {
			filename := l.Filename(test.Handle)
			if filename != test.filename {
				t.Fatalf("wrong filename, want %v, got %v", test.filename, filename)
			}
		})
	}
}

func TestRESTLayoutURLs(t *testing.T) {
	var tests = []struct {
		l   Layout
		h   backend.Handle
		fn  string
		dir string
	}{
		{
			&RESTLayout{url: "https://hostname.foo"},
			backend.Handle{Type: backend.PackFile, Name: "foobar"},
			"https://hostname.foo/data/foobar",
			"https://hostname.foo/data/",
		},
		{
			&RESTLayout{url: "https://hostname.foo:1234/prefix/repo"},
			backend.Handle{Type: backend.LockFile, Name: "foobar"},
			"https://hostname.foo:1234/prefix/repo/locks/foobar",
			"https://hostname.foo:1234/prefix/repo/locks/",
		},
		{
			&RESTLayout{url: "https://hostname.foo:1234/prefix/repo"},
			backend.Handle{Type: backend.ConfigFile, Name: "foobar"},
			"https://hostname.foo:1234/prefix/repo/config",
			"https://hostname.foo:1234/prefix/repo/",
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%T", test.l), func(t *testing.T) {
			fn := test.l.Filename(test.h)
			if fn != test.fn {
				t.Fatalf("wrong filename, want %v, got %v", test.fn, fn)
			}

			dir := test.l.Dirname(test.h)
			if dir != test.dir {
				t.Fatalf("wrong dirname, want %v, got %v", test.dir, dir)
			}
		})
	}
}
