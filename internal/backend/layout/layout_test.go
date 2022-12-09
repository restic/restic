package layout

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestDefaultLayout(t *testing.T) {
	tempdir := rtest.TempDir(t)

	var tests = []struct {
		path string
		join func(...string) string
		restic.Handle
		filename string
	}{
		{
			tempdir,
			filepath.Join,
			restic.Handle{Type: restic.PackFile, Name: "0123456"},
			filepath.Join(tempdir, "data", "01", "0123456"),
		},
		{
			tempdir,
			filepath.Join,
			restic.Handle{Type: restic.ConfigFile, Name: "CFG"},
			filepath.Join(tempdir, "config"),
		},
		{
			tempdir,
			filepath.Join,
			restic.Handle{Type: restic.SnapshotFile, Name: "123456"},
			filepath.Join(tempdir, "snapshots", "123456"),
		},
		{
			tempdir,
			filepath.Join,
			restic.Handle{Type: restic.IndexFile, Name: "123456"},
			filepath.Join(tempdir, "index", "123456"),
		},
		{
			tempdir,
			filepath.Join,
			restic.Handle{Type: restic.LockFile, Name: "123456"},
			filepath.Join(tempdir, "locks", "123456"),
		},
		{
			tempdir,
			filepath.Join,
			restic.Handle{Type: restic.KeyFile, Name: "123456"},
			filepath.Join(tempdir, "keys", "123456"),
		},
		{
			"",
			path.Join,
			restic.Handle{Type: restic.PackFile, Name: "0123456"},
			"data/01/0123456",
		},
		{
			"",
			path.Join,
			restic.Handle{Type: restic.ConfigFile, Name: "CFG"},
			"config",
		},
		{
			"",
			path.Join,
			restic.Handle{Type: restic.SnapshotFile, Name: "123456"},
			"snapshots/123456",
		},
		{
			"",
			path.Join,
			restic.Handle{Type: restic.IndexFile, Name: "123456"},
			"index/123456",
		},
		{
			"",
			path.Join,
			restic.Handle{Type: restic.LockFile, Name: "123456"},
			"locks/123456",
		},
		{
			"",
			path.Join,
			restic.Handle{Type: restic.KeyFile, Name: "123456"},
			"keys/123456",
		},
	}

	t.Run("Paths", func(t *testing.T) {
		l := &DefaultLayout{
			Path: tempdir,
			Join: filepath.Join,
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
				Path: test.path,
				Join: test.join,
			}

			filename := l.Filename(test.Handle)
			if filename != test.filename {
				t.Fatalf("wrong filename, want %v, got %v", test.filename, filename)
			}
		})
	}
}

func TestRESTLayout(t *testing.T) {
	path := rtest.TempDir(t)

	var tests = []struct {
		restic.Handle
		filename string
	}{
		{
			restic.Handle{Type: restic.PackFile, Name: "0123456"},
			filepath.Join(path, "data", "0123456"),
		},
		{
			restic.Handle{Type: restic.ConfigFile, Name: "CFG"},
			filepath.Join(path, "config"),
		},
		{
			restic.Handle{Type: restic.SnapshotFile, Name: "123456"},
			filepath.Join(path, "snapshots", "123456"),
		},
		{
			restic.Handle{Type: restic.IndexFile, Name: "123456"},
			filepath.Join(path, "index", "123456"),
		},
		{
			restic.Handle{Type: restic.LockFile, Name: "123456"},
			filepath.Join(path, "locks", "123456"),
		},
		{
			restic.Handle{Type: restic.KeyFile, Name: "123456"},
			filepath.Join(path, "keys", "123456"),
		},
	}

	l := &RESTLayout{
		Path: path,
		Join: filepath.Join,
	}

	t.Run("Paths", func(t *testing.T) {
		dirs := l.Paths()

		want := []string{
			filepath.Join(path, "data"),
			filepath.Join(path, "snapshots"),
			filepath.Join(path, "index"),
			filepath.Join(path, "locks"),
			filepath.Join(path, "keys"),
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
		h   restic.Handle
		fn  string
		dir string
	}{
		{
			&RESTLayout{URL: "https://hostname.foo", Path: "", Join: path.Join},
			restic.Handle{Type: restic.PackFile, Name: "foobar"},
			"https://hostname.foo/data/foobar",
			"https://hostname.foo/data/",
		},
		{
			&RESTLayout{URL: "https://hostname.foo:1234/prefix/repo", Path: "/", Join: path.Join},
			restic.Handle{Type: restic.LockFile, Name: "foobar"},
			"https://hostname.foo:1234/prefix/repo/locks/foobar",
			"https://hostname.foo:1234/prefix/repo/locks/",
		},
		{
			&RESTLayout{URL: "https://hostname.foo:1234/prefix/repo", Path: "/", Join: path.Join},
			restic.Handle{Type: restic.ConfigFile, Name: "foobar"},
			"https://hostname.foo:1234/prefix/repo/config",
			"https://hostname.foo:1234/prefix/repo/",
		},
		{
			&S3LegacyLayout{URL: "https://hostname.foo", Path: "/", Join: path.Join},
			restic.Handle{Type: restic.PackFile, Name: "foobar"},
			"https://hostname.foo/data/foobar",
			"https://hostname.foo/data/",
		},
		{
			&S3LegacyLayout{URL: "https://hostname.foo:1234/prefix/repo", Path: "", Join: path.Join},
			restic.Handle{Type: restic.LockFile, Name: "foobar"},
			"https://hostname.foo:1234/prefix/repo/lock/foobar",
			"https://hostname.foo:1234/prefix/repo/lock/",
		},
		{
			&S3LegacyLayout{URL: "https://hostname.foo:1234/prefix/repo", Path: "/", Join: path.Join},
			restic.Handle{Type: restic.ConfigFile, Name: "foobar"},
			"https://hostname.foo:1234/prefix/repo/config",
			"https://hostname.foo:1234/prefix/repo/",
		},
		{
			&S3LegacyLayout{URL: "", Path: "", Join: path.Join},
			restic.Handle{Type: restic.PackFile, Name: "foobar"},
			"data/foobar",
			"data/",
		},
		{
			&S3LegacyLayout{URL: "", Path: "", Join: path.Join},
			restic.Handle{Type: restic.LockFile, Name: "foobar"},
			"lock/foobar",
			"lock/",
		},
		{
			&S3LegacyLayout{URL: "", Path: "/", Join: path.Join},
			restic.Handle{Type: restic.ConfigFile, Name: "foobar"},
			"/config",
			"/",
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

func TestS3LegacyLayout(t *testing.T) {
	path := rtest.TempDir(t)

	var tests = []struct {
		restic.Handle
		filename string
	}{
		{
			restic.Handle{Type: restic.PackFile, Name: "0123456"},
			filepath.Join(path, "data", "0123456"),
		},
		{
			restic.Handle{Type: restic.ConfigFile, Name: "CFG"},
			filepath.Join(path, "config"),
		},
		{
			restic.Handle{Type: restic.SnapshotFile, Name: "123456"},
			filepath.Join(path, "snapshot", "123456"),
		},
		{
			restic.Handle{Type: restic.IndexFile, Name: "123456"},
			filepath.Join(path, "index", "123456"),
		},
		{
			restic.Handle{Type: restic.LockFile, Name: "123456"},
			filepath.Join(path, "lock", "123456"),
		},
		{
			restic.Handle{Type: restic.KeyFile, Name: "123456"},
			filepath.Join(path, "key", "123456"),
		},
	}

	l := &S3LegacyLayout{
		Path: path,
		Join: filepath.Join,
	}

	t.Run("Paths", func(t *testing.T) {
		dirs := l.Paths()

		want := []string{
			filepath.Join(path, "data"),
			filepath.Join(path, "snapshot"),
			filepath.Join(path, "index"),
			filepath.Join(path, "lock"),
			filepath.Join(path, "key"),
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

func TestDetectLayout(t *testing.T) {
	path := rtest.TempDir(t)

	var tests = []struct {
		filename string
		want     string
	}{
		{"repo-layout-default.tar.gz", "*layout.DefaultLayout"},
		{"repo-layout-s3legacy.tar.gz", "*layout.S3LegacyLayout"},
	}

	var fs = &LocalFilesystem{}
	for _, test := range tests {
		for _, fs := range []Filesystem{fs, nil} {
			t.Run(fmt.Sprintf("%v/fs-%T", test.filename, fs), func(t *testing.T) {
				rtest.SetupTarTestFixture(t, path, filepath.Join("../testdata", test.filename))

				layout, err := DetectLayout(context.TODO(), fs, filepath.Join(path, "repo"))
				if err != nil {
					t.Fatal(err)
				}

				if layout == nil {
					t.Fatal("wanted some layout, but detect returned nil")
				}

				layoutName := fmt.Sprintf("%T", layout)
				if layoutName != test.want {
					t.Fatalf("want layout %v, got %v", test.want, layoutName)
				}

				rtest.RemoveAll(t, filepath.Join(path, "repo"))
			})
		}
	}
}

func TestParseLayout(t *testing.T) {
	path := rtest.TempDir(t)

	var tests = []struct {
		layoutName        string
		defaultLayoutName string
		want              string
	}{
		{"default", "", "*layout.DefaultLayout"},
		{"s3legacy", "", "*layout.S3LegacyLayout"},
		{"", "", "*layout.DefaultLayout"},
	}

	rtest.SetupTarTestFixture(t, path, filepath.Join("..", "testdata", "repo-layout-default.tar.gz"))

	for _, test := range tests {
		t.Run(test.layoutName, func(t *testing.T) {
			layout, err := ParseLayout(context.TODO(), &LocalFilesystem{}, test.layoutName, test.defaultLayoutName, filepath.Join(path, "repo"))
			if err != nil {
				t.Fatal(err)
			}

			if layout == nil {
				t.Fatal("wanted some layout, but detect returned nil")
			}

			// test that the functions work (and don't panic)
			_ = layout.Dirname(restic.Handle{Type: restic.PackFile})
			_ = layout.Filename(restic.Handle{Type: restic.PackFile, Name: "1234"})
			_ = layout.Paths()

			layoutName := fmt.Sprintf("%T", layout)
			if layoutName != test.want {
				t.Fatalf("want layout %v, got %v", test.want, layoutName)
			}
		})
	}
}

func TestParseLayoutInvalid(t *testing.T) {
	path := rtest.TempDir(t)

	var invalidNames = []string{
		"foo", "bar", "local",
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			layout, err := ParseLayout(context.TODO(), nil, name, "", path)
			if err == nil {
				t.Fatalf("expected error not found for layout name %v, layout is %v", name, layout)
			}
		})
	}
}
