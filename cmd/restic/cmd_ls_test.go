package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestLsNodeJSON(t *testing.T) {
	for _, c := range []struct {
		path string
		restic.Node
		expect string
	}{
		// Mode is omitted when zero.
		// Permissions, by convention is "-" per mode bit
		{
			path: "/bar/baz",
			Node: restic.Node{
				Name: "baz",
				Type: "file",
				Size: 12345,
				UID:  10000000,
				GID:  20000000,

				User:  "nobody",
				Group: "nobodies",
				Links: 1,
			},
			expect: `{"name":"baz","type":"file","path":"/bar/baz","uid":10000000,"gid":20000000,"size":12345,"permissions":"----------","mtime":"0001-01-01T00:00:00Z","atime":"0001-01-01T00:00:00Z","ctime":"0001-01-01T00:00:00Z","struct_type":"node"}`,
		},

		// Even empty files get an explicit size.
		{
			path: "/foo/empty",
			Node: restic.Node{
				Name: "empty",
				Type: "file",
				Size: 0,
				UID:  1001,
				GID:  1001,

				User:  "not printed",
				Group: "not printed",
				Links: 0xF00,
			},
			expect: `{"name":"empty","type":"file","path":"/foo/empty","uid":1001,"gid":1001,"size":0,"permissions":"----------","mtime":"0001-01-01T00:00:00Z","atime":"0001-01-01T00:00:00Z","ctime":"0001-01-01T00:00:00Z","struct_type":"node"}`,
		},

		// Non-regular files do not get a size.
		// Mode is printed in decimal, including the type bits.
		{
			path: "/foo/link",
			Node: restic.Node{
				Name:       "link",
				Type:       "symlink",
				Mode:       os.ModeSymlink | 0777,
				LinkTarget: "not printed",
			},
			expect: `{"name":"link","type":"symlink","path":"/foo/link","uid":0,"gid":0,"mode":134218239,"permissions":"Lrwxrwxrwx","mtime":"0001-01-01T00:00:00Z","atime":"0001-01-01T00:00:00Z","ctime":"0001-01-01T00:00:00Z","struct_type":"node"}`,
		},

		{
			path: "/some/directory",
			Node: restic.Node{
				Name:       "directory",
				Type:       "dir",
				Mode:       os.ModeDir | 0755,
				ModTime:    time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC),
				AccessTime: time.Date(2021, 2, 3, 4, 5, 6, 7, time.UTC),
				ChangeTime: time.Date(2022, 3, 4, 5, 6, 7, 8, time.UTC),
			},
			expect: `{"name":"directory","type":"dir","path":"/some/directory","uid":0,"gid":0,"mode":2147484141,"permissions":"drwxr-xr-x","mtime":"2020-01-02T03:04:05Z","atime":"2021-02-03T04:05:06.000000007Z","ctime":"2022-03-04T05:06:07.000000008Z","struct_type":"node"}`,
		},
	} {
		buf := new(bytes.Buffer)
		enc := json.NewEncoder(buf)
		err := lsNodeJSON(enc, c.path, &c.Node)
		rtest.OK(t, err)
		rtest.Equals(t, c.expect+"\n", buf.String())

		// Sanity check: output must be valid JSON.
		var v interface{}
		err = json.NewDecoder(buf).Decode(&v)
		rtest.OK(t, err)
	}
}
