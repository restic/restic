package backend

import (
	"fmt"
	"path/filepath"
	"reflect"
	"restic"
	"restic/test"
	"sort"
	"testing"
)

func TestDefaultLayout(t *testing.T) {
	path, cleanup := test.TempDir(t)
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
			t.Fatalf("wrong paths returned, want:\v  %v\ngot:\n  %v", want, dirs)
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
