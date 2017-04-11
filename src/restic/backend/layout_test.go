package backend

import (
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"restic"
	. "restic/test"
	"sort"
	"testing"
)

func TestDefaultLayout(t *testing.T) {
	path, cleanup := TempDir(t)
	defer cleanup()

	var tests = []struct {
		restic.Handle
		filename string
	}{
		{
			restic.Handle{Type: restic.DataFile, Name: "0123456"},
			filepath.Join(path, "data", "01", "0123456"),
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

	l := &DefaultLayout{
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

		sort.Sort(sort.StringSlice(want))
		sort.Sort(sort.StringSlice(dirs))

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

func TestCloudLayout(t *testing.T) {
	path, cleanup := TempDir(t)
	defer cleanup()

	var tests = []struct {
		restic.Handle
		filename string
	}{
		{
			restic.Handle{Type: restic.DataFile, Name: "0123456"},
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

	l := &CloudLayout{
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

		sort.Sort(sort.StringSlice(want))
		sort.Sort(sort.StringSlice(dirs))

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

func TestCloudLayoutURLs(t *testing.T) {
	var tests = []struct {
		l  Layout
		h  restic.Handle
		fn string
	}{
		{
			&CloudLayout{URL: "https://hostname.foo", Path: "", Join: path.Join},
			restic.Handle{Type: restic.DataFile, Name: "foobar"},
			"https://hostname.foo/data/foobar",
		},
		{
			&CloudLayout{URL: "https://hostname.foo:1234/prefix/repo", Path: "/", Join: path.Join},
			restic.Handle{Type: restic.LockFile, Name: "foobar"},
			"https://hostname.foo:1234/prefix/repo/locks/foobar",
		},
		{
			&CloudLayout{URL: "https://hostname.foo:1234/prefix/repo", Path: "/", Join: path.Join},
			restic.Handle{Type: restic.ConfigFile, Name: "foobar"},
			"https://hostname.foo:1234/prefix/repo/config",
		},
		{
			&S3Layout{URL: "https://hostname.foo", Path: "/", Join: path.Join},
			restic.Handle{Type: restic.DataFile, Name: "foobar"},
			"https://hostname.foo/data/foobar",
		},
		{
			&S3Layout{URL: "https://hostname.foo:1234/prefix/repo", Path: "", Join: path.Join},
			restic.Handle{Type: restic.LockFile, Name: "foobar"},
			"https://hostname.foo:1234/prefix/repo/lock/foobar",
		},
		{
			&S3Layout{URL: "https://hostname.foo:1234/prefix/repo", Path: "/", Join: path.Join},
			restic.Handle{Type: restic.ConfigFile, Name: "foobar"},
			"https://hostname.foo:1234/prefix/repo/config",
		},
	}

	for _, test := range tests {
		t.Run("cloud", func(t *testing.T) {
			res := test.l.Filename(test.h)
			if res != test.fn {
				t.Fatalf("wrong filename, want %v, got %v", test.fn, res)
			}
		})
	}
}

func TestS3Layout(t *testing.T) {
	path, cleanup := TempDir(t)
	defer cleanup()

	var tests = []struct {
		restic.Handle
		filename string
	}{
		{
			restic.Handle{Type: restic.DataFile, Name: "0123456"},
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

	l := &S3Layout{
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

		sort.Sort(sort.StringSlice(want))
		sort.Sort(sort.StringSlice(dirs))

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
	path, cleanup := TempDir(t)
	defer cleanup()

	var tests = []struct {
		filename string
		want     string
	}{
		{"repo-layout-local.tar.gz", "*backend.DefaultLayout"},
		{"repo-layout-cloud.tar.gz", "*backend.CloudLayout"},
		{"repo-layout-s3-old.tar.gz", "*backend.S3Layout"},
	}

	var fs = &LocalFilesystem{}
	for _, test := range tests {
		for _, fs := range []Filesystem{fs, nil} {
			t.Run(fmt.Sprintf("%v/fs-%T", test.filename, fs), func(t *testing.T) {
				SetupTarTestFixture(t, path, filepath.Join("testdata", test.filename))

				layout, err := DetectLayout(fs, filepath.Join(path, "repo"))
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

				RemoveAll(t, filepath.Join(path, "repo"))
			})
		}
	}
}

func TestParseLayout(t *testing.T) {
	path, cleanup := TempDir(t)
	defer cleanup()

	var tests = []struct {
		layoutName        string
		defaultLayoutName string
		want              string
	}{
		{"default", "", "*backend.DefaultLayout"},
		{"cloud", "", "*backend.CloudLayout"},
		{"s3", "", "*backend.S3Layout"},
		{"", "", "*backend.CloudLayout"},
	}

	SetupTarTestFixture(t, path, filepath.Join("testdata", "repo-layout-cloud.tar.gz"))

	for _, test := range tests {
		t.Run(test.layoutName, func(t *testing.T) {
			layout, err := ParseLayout(&LocalFilesystem{}, test.layoutName, test.defaultLayoutName, filepath.Join(path, "repo"))
			if err != nil {
				t.Fatal(err)
			}

			if layout == nil {
				t.Fatal("wanted some layout, but detect returned nil")
			}

			// test that the functions work (and don't panic)
			_ = layout.Dirname(restic.Handle{Type: restic.DataFile})
			_ = layout.Filename(restic.Handle{Type: restic.DataFile, Name: "1234"})
			_ = layout.Paths()

			layoutName := fmt.Sprintf("%T", layout)
			if layoutName != test.want {
				t.Fatalf("want layout %v, got %v", test.want, layoutName)
			}
		})
	}
}

func TestParseLayoutInvalid(t *testing.T) {
	path, cleanup := TempDir(t)
	defer cleanup()

	var invalidNames = []string{
		"foo", "bar", "local",
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			layout, err := ParseLayout(nil, name, "", path)
			if err == nil {
				t.Fatalf("expected error not found for layout name %v, layout is %v", name, layout)
			}
		})
	}
}
