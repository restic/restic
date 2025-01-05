package fs

import (
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
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func BenchmarkNodeFromFileInfo(t *testing.B) {
	tempfile, err := os.CreateTemp(t.TempDir(), "restic-test-temp-")
	rtest.OK(t, err)
	path := tempfile.Name()
	rtest.OK(t, tempfile.Close())

	fs := Local{}
	f, err := fs.OpenFile(path, O_NOFOLLOW, true)
	rtest.OK(t, err)
	_, err = f.Stat()
	rtest.OK(t, err)

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		_, err := f.ToNode(false)
		rtest.OK(t, err)
	}

	rtest.OK(t, f.Close())
}

func parseTime(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05.999", s)
	if err != nil {
		panic(err)
	}

	return t.Local()
}

var nodeTests = []restic.Node{
	{
		Name:       "testFile",
		Type:       restic.NodeTypeFile,
		Content:    restic.IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0604,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	{
		Name:       "testSuidFile",
		Type:       restic.NodeTypeFile,
		Content:    restic.IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0755 | os.ModeSetuid,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	{
		Name:       "testSuidFile2",
		Type:       restic.NodeTypeFile,
		Content:    restic.IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0755 | os.ModeSetgid,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	{
		Name:       "testSticky",
		Type:       restic.NodeTypeFile,
		Content:    restic.IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0755 | os.ModeSticky,
		ModTime:    parseTime("2015-05-14 21:07:23.111"),
		AccessTime: parseTime("2015-05-14 21:07:24.222"),
		ChangeTime: parseTime("2015-05-14 21:07:25.333"),
	},
	{
		Name:       "testDir",
		Type:       restic.NodeTypeDir,
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
		Type:       restic.NodeTypeSymlink,
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
		Type:       restic.NodeTypeFile,
		Content:    restic.IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0604,
		ModTime:    parseTime("2005-05-14 21:07:03.111"),
		AccessTime: parseTime("2005-05-14 21:07:04.222"),
		ChangeTime: parseTime("2005-05-14 21:07:05.333"),
	},
	{
		Name:       "testDir",
		Type:       restic.NodeTypeDir,
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
		Type:       restic.NodeTypeFile,
		Content:    restic.IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0604,
		ModTime:    parseTime("2005-05-14 21:07:03.111"),
		AccessTime: parseTime("2005-05-14 21:07:04.222"),
		ChangeTime: parseTime("2005-05-14 21:07:05.333"),
		ExtendedAttributes: []restic.ExtendedAttribute{
			{Name: "user.foo", Value: []byte("bar")},
		},
	},
	{
		Name:       "testXattrDir",
		Type:       restic.NodeTypeDir,
		Subtree:    nil,
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0750 | os.ModeDir,
		ModTime:    parseTime("2005-05-14 21:07:03.111"),
		AccessTime: parseTime("2005-05-14 21:07:04.222"),
		ChangeTime: parseTime("2005-05-14 21:07:05.333"),
		ExtendedAttributes: []restic.ExtendedAttribute{
			{Name: "user.foo", Value: []byte("bar")},
		},
	},
	{
		Name:       "testXattrFileMacOSResourceFork",
		Type:       restic.NodeTypeFile,
		Content:    restic.IDs{},
		UID:        uint32(os.Getuid()),
		GID:        uint32(os.Getgid()),
		Mode:       0604,
		ModTime:    parseTime("2005-05-14 21:07:03.111"),
		AccessTime: parseTime("2005-05-14 21:07:04.222"),
		ChangeTime: parseTime("2005-05-14 21:07:05.333"),
		ExtendedAttributes: []restic.ExtendedAttribute{
			{Name: "com.apple.ResourceFork", Value: []byte("bar")},
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
				for _, attr := range test.ExtendedAttributes {
					if strings.HasPrefix(attr.Name, "com.apple.") && runtime.GOOS != "darwin" {
						t.Skipf("attr %v only relevant on macOS", attr.Name)
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
			rtest.OK(t, NodeCreateAt(&test, nodePath))
			rtest.OK(t, NodeRestoreMetadata(&test, nodePath, func(msg string) { rtest.OK(t, fmt.Errorf("Warning triggered for path: %s: %s", nodePath, msg)) }))

			fs := &Local{}
			meta, err := fs.OpenFile(nodePath, O_NOFOLLOW, true)
			rtest.OK(t, err)
			n2, err := meta.ToNode(false)
			rtest.OK(t, err)
			n3, err := meta.ToNode(true)
			rtest.OK(t, err)
			rtest.OK(t, meta.Close())
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
				if test.Type != restic.NodeTypeSymlink {
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

func AssertFsTimeEqual(t *testing.T, label string, nodeType restic.NodeType, t1 time.Time, t2 time.Time) {
	var equal bool

	// Go currently doesn't support setting timestamps of symbolic links on darwin and bsd
	if nodeType == restic.NodeTypeSymlink {
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

func TestNodeRestoreMetadataError(t *testing.T) {
	tempdir := t.TempDir()

	node := &nodeTests[0]
	nodePath := filepath.Join(tempdir, node.Name)

	// This will fail because the target file does not exist
	err := NodeRestoreMetadata(node, nodePath, func(msg string) { rtest.OK(t, fmt.Errorf("Warning triggered for path: %s: %s", nodePath, msg)) })
	rtest.Assert(t, errors.Is(err, os.ErrNotExist), "failed for an unexpected reason")
}
