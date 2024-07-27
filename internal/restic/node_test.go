package restic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/test"
	rtest "github.com/restic/restic/internal/test"
)

func BenchmarkNodeFillUser(t *testing.B) {
	tempfile, err := os.CreateTemp("", "restic-test-temp-")
	if err != nil {
		t.Fatal(err)
	}

	fi, err := tempfile.Stat()
	if err != nil {
		t.Fatal(err)
	}

	path := tempfile.Name()

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		_, err := NodeFromFileInfo(path, fi, false)
		rtest.OK(t, err)
	}

	rtest.OK(t, tempfile.Close())
	rtest.RemoveAll(t, tempfile.Name())
}

func BenchmarkNodeFromFileInfo(t *testing.B) {
	tempfile, err := os.CreateTemp("", "restic-test-temp-")
	if err != nil {
		t.Fatal(err)
	}

	fi, err := tempfile.Stat()
	if err != nil {
		t.Fatal(err)
	}

	path := tempfile.Name()

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		_, err := NodeFromFileInfo(path, fi, false)
		if err != nil {
			t.Fatal(err)
		}
	}

	rtest.OK(t, tempfile.Close())
	rtest.RemoveAll(t, tempfile.Name())
}

func parseTime(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05.999", s)
	if err != nil {
		panic(err)
	}

	return t.Local()
}

var nodeTests = []Node{
	{
		Name:       "testFile",
		Type:       "file",
		Content:    IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0604,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	{
		Name:       "testSuidFile",
		Type:       "file",
		Content:    IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0755 | os.ModeSetuid,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	{
		Name:       "testSuidFile2",
		Type:       "file",
		Content:    IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0755 | os.ModeSetgid,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	{
		Name:       "testSticky",
		Type:       "file",
		Content:    IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0755 | os.ModeSticky,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	{
		Name:       "testDir",
		Type:       "dir",
		Subtree:    nil,
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0750 | os.ModeDir,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	{
		Name:       "testSymlink",
		Type:       "symlink",
		LinkTarget: "invalid",
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0777 | os.ModeSymlink,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},

	// include "testFile" and "testDir" again with slightly different
	// metadata, so we can test if CreateAt works with pre-existing files.
	{
		Name:       "testFile",
		Type:       "file",
		Content:    IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0604,
		ModTime:    parseTime("2005-05-14 21:07:03.111"),
		AccessTime: parseTime("2005-05-14 21:07:04.222"),
		ChangeTime: parseTime("2005-05-14 21:07:05.333"),
	},
	{
		Name:       "testDir",
		Type:       "dir",
		Subtree:    nil,
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0750 | os.ModeDir,
		ModTime:    parseTime("2005-05-14 21:07:03.111"),
		AccessTime: parseTime("2005-05-14 21:07:04.222"),
		ChangeTime: parseTime("2005-05-14 21:07:05.333"),
	},
	{
		Name:       "testXattrFile",
		Type:       "file",
		Content:    IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0604,
		ModTime:    parseTime("2005-05-14 21:07:03.111"),
		AccessTime: parseTime("2005-05-14 21:07:04.222"),
		ChangeTime: parseTime("2005-05-14 21:07:05.333"),
		ExtendedAttributes: []ExtendedAttribute{
			{"user.foo", []byte("bar")},
		},
	},
	{
		Name:       "testXattrDir",
		Type:       "dir",
		Subtree:    nil,
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0750 | os.ModeDir,
		ModTime:    parseTime("2005-05-14 21:07:03.111"),
		AccessTime: parseTime("2005-05-14 21:07:04.222"),
		ChangeTime: parseTime("2005-05-14 21:07:05.333"),
		ExtendedAttributes: []ExtendedAttribute{
			{"user.foo", []byte("bar")},
		},
	},
}

func TestNodeRestoreAt(t *testing.T) {
	tempdir := t.TempDir()

	for _, test := range nodeTests {
		t.Run("", func(t *testing.T) {
			var nodePath string
			if test.ExtendedAttributes != nil {
				if runtime.GOOS == "windows" {
					// In windows extended attributes are case insensitive and windows returns
					// the extended attributes in UPPER case.
					// Update the tests to use UPPER case xattr names for windows.
					extAttrArr := test.ExtendedAttributes
					// Iterate through the array using pointers
					for i := 0; i < len(extAttrArr); i++ {
						extAttrArr[i].Name = strings.ToUpper(extAttrArr[i].Name)
					}
				}

				// tempdir might be backed by a filesystem that does not support
				// extended attributes
				nodePath = test.Name
				defer func() {
					_ = os.Remove(nodePath)
				}()
			} else {
				nodePath = filepath.Join(tempdir, test.Name)
			}
			rtest.OK(t, test.CreateAt(context.TODO(), nodePath, nil))
			rtest.OK(t, test.RestoreMetadata(nodePath, func(msg string) { rtest.OK(t, fmt.Errorf("Warning triggered for path: %s: %s", nodePath, msg)) }))

			if test.Type == "dir" {
				rtest.OK(t, test.RestoreTimestamps(nodePath))
			}

			fi, err := os.Lstat(nodePath)
			rtest.OK(t, err)

			n2, err := NodeFromFileInfo(nodePath, fi, false)
			rtest.OK(t, err)
			n3, err := NodeFromFileInfo(nodePath, fi, true)
			rtest.OK(t, err)
			rtest.Assert(t, n2.Equals(*n3), "unexpected node info mismatch %v", cmp.Diff(n2, n3))

			rtest.Assert(t, test.Name == n2.Name,
				"%v: name doesn't match (%v != %v)", test.Type, test.Name, n2.Name)
			rtest.Assert(t, test.Type == n2.Type,
				"%v: type doesn't match (%v != %v)", test.Type, test.Type, n2.Type)
			rtest.Assert(t, test.Size == n2.Size,
				"%v: size doesn't match (%v != %v)", test.Size, test.Size, n2.Size)

			if runtime.GOOS != "windows" {
				rtest.Assert(t, test.UID == n2.UID,
					"%v: UID doesn't match (%v != %v)", test.Type, test.UID, n2.UID)
				rtest.Assert(t, test.GID == n2.GID,
					"%v: GID doesn't match (%v != %v)", test.Type, test.GID, n2.GID)
				if test.Type != "symlink" {
					// On OpenBSD only root can set sticky bit (see sticky(8)).
					if runtime.GOOS != "openbsd" && runtime.GOOS != "netbsd" && runtime.GOOS != "solaris" && test.Name == "testSticky" {
						rtest.Assert(t, test.Mode == n2.Mode,
							"%v: mode doesn't match (0%o != 0%o)", test.Type, test.Mode, n2.Mode)
					}
				}
			}

			AssertFsTimeEqual(t, "AccessTime", test.Type, test.AccessTime, n2.AccessTime)
			AssertFsTimeEqual(t, "ModTime", test.Type, test.ModTime, n2.ModTime)
			if len(n2.ExtendedAttributes) == 0 {
				n2.ExtendedAttributes = nil
			}
			rtest.Assert(t, reflect.DeepEqual(test.ExtendedAttributes, n2.ExtendedAttributes),
				"%v: xattrs don't match (%v != %v)", test.Name, test.ExtendedAttributes, n2.ExtendedAttributes)
		})
	}
}

func AssertFsTimeEqual(t *testing.T, label string, nodeType string, t1 time.Time, t2 time.Time) {
	var equal bool

	// Go currently doesn't support setting timestamps of symbolic links on darwin and bsd
	if nodeType == "symlink" {
		switch runtime.GOOS {
		case "darwin", "freebsd", "openbsd", "netbsd", "solaris":
			return
		}
	}

	switch runtime.GOOS {
	case "darwin":
		// HFS+ timestamps don't support sub-second precision,
		// see https://en.wikipedia.org/wiki/Comparison_of_file_systems
		diff := int(t1.Sub(t2).Seconds())
		equal = diff == 0
	default:
		equal = t1.Equal(t2)
	}

	rtest.Assert(t, equal, "%s: %s doesn't match (%v != %v)", label, nodeType, t1, t2)
}

func parseTimeNano(t testing.TB, s string) time.Time {
	// 2006-01-02T15:04:05.999999999Z07:00
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("error parsing %q: %v", s, err)
	}
	return ts
}

func TestFixTime(t *testing.T) {
	// load UTC location
	utc, err := time.LoadLocation("")
	if err != nil {
		t.Fatal(err)
	}

	var tests = []struct {
		src, want time.Time
	}{
		{
			src:  parseTimeNano(t, "2006-01-02T15:04:05.999999999+07:00"),
			want: parseTimeNano(t, "2006-01-02T15:04:05.999999999+07:00"),
		},
		{
			src:  time.Date(0, 1, 2, 3, 4, 5, 6, utc),
			want: parseTimeNano(t, "0000-01-02T03:04:05.000000006+00:00"),
		},
		{
			src:  time.Date(-2, 1, 2, 3, 4, 5, 6, utc),
			want: parseTimeNano(t, "0000-01-02T03:04:05.000000006+00:00"),
		},
		{
			src:  time.Date(12345, 1, 2, 3, 4, 5, 6, utc),
			want: parseTimeNano(t, "9999-01-02T03:04:05.000000006+00:00"),
		},
		{
			src:  time.Date(9999, 1, 2, 3, 4, 5, 6, utc),
			want: parseTimeNano(t, "9999-01-02T03:04:05.000000006+00:00"),
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			res := FixTime(test.src)
			if !res.Equal(test.want) {
				t.Fatalf("wrong result for %v, want:\n  %v\ngot:\n  %v", test.src, test.want, res)
			}
		})
	}
}

func TestSymlinkSerialization(t *testing.T) {
	for _, link := range []string{
		"válîd \t Üñi¢òde \n śẗŕinǵ",
		string([]byte{0, 1, 2, 0xfa, 0xfb, 0xfc}),
	} {
		n := Node{
			LinkTarget: link,
		}
		ser, err := json.Marshal(n)
		test.OK(t, err)
		var n2 Node
		err = json.Unmarshal(ser, &n2)
		test.OK(t, err)
		fmt.Println(string(ser))

		test.Equals(t, n.LinkTarget, n2.LinkTarget)
	}
}

func TestSymlinkSerializationFormat(t *testing.T) {
	for _, d := range []struct {
		ser        string
		linkTarget string
	}{
		{`{"linktarget":"test"}`, "test"},
		{`{"linktarget":"\u0000\u0001\u0002\ufffd\ufffd\ufffd","linktarget_raw":"AAEC+vv8"}`, string([]byte{0, 1, 2, 0xfa, 0xfb, 0xfc})},
	} {
		var n2 Node
		err := json.Unmarshal([]byte(d.ser), &n2)
		test.OK(t, err)
		test.Equals(t, d.linkTarget, n2.LinkTarget)
		test.Assert(t, n2.LinkTargetRaw == nil, "quoted link target is just a helper field and must be unset after decoding")
	}
}

func TestNodeRestoreMetadataError(t *testing.T) {
	tempdir := t.TempDir()

	node := nodeTests[0]
	nodePath := filepath.Join(tempdir, node.Name)

	// This will fail because the target file does not exist
	err := node.RestoreMetadata(nodePath, func(msg string) { rtest.OK(t, fmt.Errorf("Warning triggered for path: %s: %s", nodePath, msg)) })
	test.Assert(t, errors.Is(err, os.ErrNotExist), "failed for an unexpected reason")
}
