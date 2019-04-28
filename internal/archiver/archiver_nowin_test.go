// +build !windows

package archiver

import (
	"context"
	"os"
	"syscall"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/restic/restic/internal/checker"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

func TestMetadataChanged(t *testing.T) {
	files := TestDir{
		"testfile": TestFile{
			Content: "foo bar test file",
		},
	}

	tempdir, repo, cleanup := prepareTempdirRepoSrc(t, files)
	defer cleanup()

	back := fs.TestChdir(t, tempdir)
	defer back()

	// get metadata
	fi := lstat(t, "testfile")
	want, err := restic.NodeFromFileInfo("testfile", fi)
	if err != nil {
		t.Fatal(err)
	}

	fs := &StatFS{
		FS: fs.Local{},
		OverrideLstat: map[string]os.FileInfo{
			"testfile": fi,
		},
	}

	snapshotID, node2 := snapshot(t, repo, fs, restic.ID{}, "testfile")

	// set some values so we can then compare the nodes
	want.Content = node2.Content
	want.Path = ""
	want.ExtendedAttributes = nil

	// make sure that metadata was recorded successfully
	if !cmp.Equal(want, node2) {
		t.Fatalf("metadata does not match:\n%v", cmp.Diff(want, node2))
	}

	// modify the mode
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if ok {
		// change a few values
		stat.Mode = 0400
		stat.Uid = 1234
		stat.Gid = 1235

		// wrap the os.FileInfo so we can return a modified stat_t
		fi = wrappedFileInfo{
			FileInfo: fi,
			sys:      stat,
			mode:     0400,
		}
		fs.OverrideLstat["testfile"] = fi
	} else {
		// skip the test on this platform
		t.Skipf("unable to modify os.FileInfo, Sys() returned %T", fi.Sys())
	}

	want, err = restic.NodeFromFileInfo("testfile", fi)
	if err != nil {
		t.Fatal(err)
	}

	// make another snapshot
	snapshotID, node3 := snapshot(t, repo, fs, snapshotID, "testfile")

	// set some values so we can then compare the nodes
	want.Content = node2.Content
	want.Path = ""
	want.ExtendedAttributes = nil

	// make sure that metadata was recorded successfully
	if !cmp.Equal(want, node3) {
		t.Fatalf("metadata does not match:\n%v", cmp.Diff(want, node2))
	}

	// make sure the content matches
	TestEnsureFileContent(context.Background(), t, repo, "testfile", node3, files["testfile"].(TestFile))

	checker.TestCheckRepo(t, repo)
}
