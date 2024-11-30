package fs

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

type fsLocalMetadataTestcase struct {
	name     string
	follow   bool
	setup    func(t *testing.T, path string)
	nodeType restic.NodeType
}

func TestFSLocalMetadata(t *testing.T) {
	for _, test := range []fsLocalMetadataTestcase{
		{
			name: "file",
			setup: func(t *testing.T, path string) {
				rtest.OK(t, os.WriteFile(path, []byte("example"), 0o600))
			},
			nodeType: restic.NodeTypeFile,
		},
		{
			name: "directory",
			setup: func(t *testing.T, path string) {
				rtest.OK(t, os.Mkdir(path, 0o600))
			},
			nodeType: restic.NodeTypeDir,
		},
		{
			name: "symlink",
			setup: func(t *testing.T, path string) {
				rtest.OK(t, os.Symlink(path+"old", path))
			},
			nodeType: restic.NodeTypeSymlink,
		},
		{
			name:   "symlink file",
			follow: true,
			setup: func(t *testing.T, path string) {
				rtest.OK(t, os.WriteFile(path+"file", []byte("example"), 0o600))
				rtest.OK(t, os.Symlink(path+"file", path))
			},
			nodeType: restic.NodeTypeFile,
		},
	} {
		runFSLocalTestcase(t, test)
	}
}

func runFSLocalTestcase(t *testing.T, test fsLocalMetadataTestcase) {
	t.Run(test.name, func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "item")
		test.setup(t, path)

		testFs := &Local{}
		flags := 0
		if !test.follow {
			flags |= O_NOFOLLOW
		}
		f, err := testFs.OpenFile(path, flags, true)
		rtest.OK(t, err)
		checkMetadata(t, f, path, test.follow, test.nodeType)
		rtest.OK(t, f.Close())
	})

}

func checkMetadata(t *testing.T, f File, path string, follow bool, nodeType restic.NodeType) {
	fi, err := f.Stat()
	rtest.OK(t, err)
	var fi2 os.FileInfo
	if follow {
		fi2, err = os.Stat(path)
	} else {
		fi2, err = os.Lstat(path)
	}
	rtest.OK(t, err)
	assertFIEqual(t, fi2, fi)

	node, err := f.ToNode(false)
	rtest.OK(t, err)

	// ModTime is likely unique per file, thus it provides a good indication that it is from the correct file
	rtest.Equals(t, fi.ModTime(), node.ModTime, "node ModTime")
	rtest.Equals(t, nodeType, node.Type, "node Type")
}

func assertFIEqual(t *testing.T, want os.FileInfo, got os.FileInfo) {
	t.Helper()
	rtest.Equals(t, want.Name(), got.Name(), "Name")
	rtest.Equals(t, want.IsDir(), got.IsDir(), "IsDir")
	rtest.Equals(t, want.ModTime(), got.ModTime(), "ModTime")
	rtest.Equals(t, want.Mode(), got.Mode(), "Mode")
	rtest.Equals(t, want.Size(), got.Size(), "Size")
}

func TestFSLocalRead(t *testing.T) {
	testFSLocalRead(t, false)
	testFSLocalRead(t, true)
}

func testFSLocalRead(t *testing.T, makeReadable bool) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "item")
	testdata := "example"
	rtest.OK(t, os.WriteFile(path, []byte(testdata), 0o600))

	f := openReadable(t, path, makeReadable)
	checkMetadata(t, f, path, false, restic.NodeTypeFile)

	data, err := io.ReadAll(f)
	rtest.OK(t, err)
	rtest.Equals(t, testdata, string(data), "file content mismatch")

	rtest.OK(t, f.Close())
}

func openReadable(t *testing.T, path string, useMakeReadable bool) File {
	testFs := &Local{}
	f, err := testFs.OpenFile(path, O_NOFOLLOW, useMakeReadable)
	rtest.OK(t, err)
	if useMakeReadable {
		// file was opened as metadataOnly. open for reading
		rtest.OK(t, f.MakeReadable())
	}
	return f
}

func TestFSLocalReaddir(t *testing.T) {
	testFSLocalReaddir(t, false)
	testFSLocalReaddir(t, true)
}

func testFSLocalReaddir(t *testing.T, makeReadable bool) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "item")
	rtest.OK(t, os.Mkdir(path, 0o700))
	entries := []string{"testfile"}
	rtest.OK(t, os.WriteFile(filepath.Join(path, entries[0]), []byte("example"), 0o600))

	f := openReadable(t, path, makeReadable)
	checkMetadata(t, f, path, false, restic.NodeTypeDir)

	names, err := f.Readdirnames(-1)
	rtest.OK(t, err)
	slices.Sort(names)
	rtest.Equals(t, entries, names, "directory content mismatch")

	rtest.OK(t, f.Close())
}

func TestFSLocalReadableRace(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "item")
	testdata := "example"
	rtest.OK(t, os.WriteFile(path, []byte(testdata), 0o600))

	testFs := &Local{}
	f, err := testFs.OpenFile(path, O_NOFOLLOW, true)
	rtest.OK(t, err)

	pathNew := path + "new"
	rtest.OK(t, os.Rename(path, pathNew))

	err = f.MakeReadable()
	if err == nil {
		// a file handle based implementation should still work
		checkMetadata(t, f, pathNew, false, restic.NodeTypeFile)

		data, err := io.ReadAll(f)
		rtest.OK(t, err)
		rtest.Equals(t, testdata, string(data), "file content mismatch")
	}

	rtest.OK(t, f.Close())
}

func TestFSLocalTypeChange(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "item")
	testdata := "example"
	rtest.OK(t, os.WriteFile(path, []byte(testdata), 0o600))

	testFs := &Local{}
	f, err := testFs.OpenFile(path, O_NOFOLLOW, true)
	rtest.OK(t, err)
	// cache metadata
	_, err = f.Stat()
	rtest.OK(t, err)

	pathNew := path + "new"
	// rename instead of unlink to let the test also work on windows
	rtest.OK(t, os.Rename(path, pathNew))

	rtest.OK(t, os.Mkdir(path, 0o700))
	rtest.OK(t, f.MakeReadable())

	fi, err := f.Stat()
	rtest.OK(t, err)
	if !fi.IsDir() {
		// a file handle based implementation should still reference the file
		checkMetadata(t, f, pathNew, false, restic.NodeTypeFile)

		data, err := io.ReadAll(f)
		rtest.OK(t, err)
		rtest.Equals(t, testdata, string(data), "file content mismatch")
	}
	// else:
	// path-based implementation
	// nothing to test here. stat returned the new file type

	rtest.OK(t, f.Close())
}
