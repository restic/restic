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

type lsTestNode struct {
	path string
	restic.Node
}

var lsTestNodes = []lsTestNode{
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
	},

	// Test encoding of setuid/setgid/sticky bit
	{
		path: "/some/sticky",
		Node: restic.Node{
			Name: "sticky",
			Type: "dir",
			Mode: os.ModeDir | 0755 | os.ModeSetuid | os.ModeSetgid | os.ModeSticky,
		},
	},
}

func TestLsNodeJSON(t *testing.T) {
	for i, expect := range []string{
		`{"name":"baz","type":"file","path":"/bar/baz","uid":10000000,"gid":20000000,"size":12345,"permissions":"----------","mtime":"0001-01-01T00:00:00Z","atime":"0001-01-01T00:00:00Z","ctime":"0001-01-01T00:00:00Z","message_type":"node","struct_type":"node"}`,
		`{"name":"empty","type":"file","path":"/foo/empty","uid":1001,"gid":1001,"size":0,"permissions":"----------","mtime":"0001-01-01T00:00:00Z","atime":"0001-01-01T00:00:00Z","ctime":"0001-01-01T00:00:00Z","message_type":"node","struct_type":"node"}`,
		`{"name":"link","type":"symlink","path":"/foo/link","uid":0,"gid":0,"mode":134218239,"permissions":"Lrwxrwxrwx","mtime":"0001-01-01T00:00:00Z","atime":"0001-01-01T00:00:00Z","ctime":"0001-01-01T00:00:00Z","message_type":"node","struct_type":"node"}`,
		`{"name":"directory","type":"dir","path":"/some/directory","uid":0,"gid":0,"mode":2147484141,"permissions":"drwxr-xr-x","mtime":"2020-01-02T03:04:05Z","atime":"2021-02-03T04:05:06.000000007Z","ctime":"2022-03-04T05:06:07.000000008Z","message_type":"node","struct_type":"node"}`,
		`{"name":"sticky","type":"dir","path":"/some/sticky","uid":0,"gid":0,"mode":2161115629,"permissions":"dugtrwxr-xr-x","mtime":"0001-01-01T00:00:00Z","atime":"0001-01-01T00:00:00Z","ctime":"0001-01-01T00:00:00Z","message_type":"node","struct_type":"node"}`,
	} {
		c := lsTestNodes[i]
		buf := new(bytes.Buffer)
		enc := json.NewEncoder(buf)
		err := lsNodeJSON(enc, c.path, &c.Node)
		rtest.OK(t, err)
		rtest.Equals(t, expect+"\n", buf.String())

		// Sanity check: output must be valid JSON.
		var v interface{}
		err = json.NewDecoder(buf).Decode(&v)
		rtest.OK(t, err)
	}
}

func TestLsNcduNode(t *testing.T) {
	for i, expect := range []string{
		`{"name":"baz","asize":12345,"dsize":12800,"dev":0,"ino":0,"nlink":1,"notreg":false,"uid":10000000,"gid":20000000,"mode":0,"mtime":0}`,
		`{"name":"empty","asize":0,"dsize":0,"dev":0,"ino":0,"nlink":3840,"notreg":false,"uid":1001,"gid":1001,"mode":0,"mtime":0}`,
		`{"name":"link","asize":0,"dsize":0,"dev":0,"ino":0,"nlink":0,"notreg":true,"uid":0,"gid":0,"mode":511,"mtime":0}`,
		`{"name":"directory","asize":0,"dsize":0,"dev":0,"ino":0,"nlink":0,"notreg":false,"uid":0,"gid":0,"mode":493,"mtime":1577934245}`,
		`{"name":"sticky","asize":0,"dsize":0,"dev":0,"ino":0,"nlink":0,"notreg":false,"uid":0,"gid":0,"mode":4077,"mtime":0}`,
	} {
		c := lsTestNodes[i]
		out, err := lsNcduNode(c.path, &c.Node)
		rtest.OK(t, err)
		rtest.Equals(t, expect, string(out))

		// Sanity check: output must be valid JSON.
		var v interface{}
		err = json.Unmarshal(out, &v)
		rtest.OK(t, err)
	}
}

func TestLsNcdu(t *testing.T) {
	var buf bytes.Buffer
	printer := &ncduLsPrinter{
		out: &buf,
	}
	modTime := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)

	printer.Snapshot(&restic.Snapshot{
		Hostname: "host",
		Paths:    []string{"/example"},
	})
	printer.Node("/directory", &restic.Node{
		Type:    "dir",
		Name:    "directory",
		ModTime: modTime,
	}, false)
	printer.Node("/directory/data", &restic.Node{
		Type:    "file",
		Name:    "data",
		Size:    42,
		ModTime: modTime,
	}, false)
	printer.LeaveDir("/directory")
	printer.Node("/file", &restic.Node{
		Type:    "file",
		Name:    "file",
		Size:    12345,
		ModTime: modTime,
	}, false)
	printer.Close()

	rtest.Equals(t, `[1, 2, {"time":"0001-01-01T00:00:00Z","tree":null,"paths":["/example"],"hostname":"host"}, [{"name":"/"},
  [
    {"name":"directory","asize":0,"dsize":0,"dev":0,"ino":0,"nlink":0,"notreg":false,"uid":0,"gid":0,"mode":0,"mtime":1577934245},
    {"name":"data","asize":42,"dsize":512,"dev":0,"ino":0,"nlink":0,"notreg":false,"uid":0,"gid":0,"mode":0,"mtime":1577934245}
  ],
  {"name":"file","asize":12345,"dsize":12800,"dev":0,"ino":0,"nlink":0,"notreg":false,"uid":0,"gid":0,"mode":0,"mtime":1577934245}
]
]
`, buf.String())
}
