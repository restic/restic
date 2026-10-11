//go:build !windows

package restorer

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/restic/restic/internal/repository"
	rtest "github.com/restic/restic/internal/test"
)

func TestRestorerRestoreEmptyHardlinkedFields(t *testing.T) {
	repo := repository.TestRepository(t)

	sn, _ := saveSnapshot(t, repo, Snapshot{
		Nodes: map[string]Node{
			"dirtest": Dir{
				Nodes: map[string]Node{
					"file1": File{Links: 2, Inode: 1},
					"file2": File{Links: 2, Inode: 1},
				},
			},
		},
	}, noopGetGenericAttributes)

	res := NewRestorer(repo, sn, Options{})

	tempdir := rtest.TempDir(t)
	ctx := t.Context()

	_, err := res.RestoreTo(ctx, tempdir)
	rtest.OK(t, err)

	f1, err := os.Stat(filepath.Join(tempdir, "dirtest/file1"))
	rtest.OK(t, err)
	rtest.Equals(t, int64(0), f1.Size())
	s1, ok1 := f1.Sys().(*syscall.Stat_t)

	f2, err := os.Stat(filepath.Join(tempdir, "dirtest/file2"))
	rtest.OK(t, err)
	rtest.Equals(t, int64(0), f2.Size())
	s2, ok2 := f2.Sys().(*syscall.Stat_t)

	if ok1 && ok2 {
		rtest.Equals(t, s1.Ino, s2.Ino)
	}
}

func getBlockCount(t *testing.T, filename string) int64 {
	fi, err := os.Stat(filename)
	rtest.OK(t, err)
	st := fi.Sys().(*syscall.Stat_t)
	if st == nil {
		return -1
	}
	return st.Blocks
}

func TestRestorerProgressBar(t *testing.T) {
	testRestorerProgressBar(t, false)
}

func TestRestorerProgressBarDryRun(t *testing.T) {
	testRestorerProgressBar(t, true)
}

func testRestorerProgressBar(t *testing.T, dryRun bool) {
	repo := repository.TestRepository(t)

	sn, _ := saveSnapshot(t, repo, Snapshot{
		Nodes: map[string]Node{
			"dirtest": Dir{
				Nodes: map[string]Node{
					"file1": File{Links: 2, Inode: 1, Data: "foo"},
					"file2": File{Links: 2, Inode: 1, Data: "foo"},
				},
			},
			"file2": File{Links: 1, Inode: 2, Data: "example"},
		},
	}, noopGetGenericAttributes)

	progress := newTestProgress()
	res := NewRestorer(repo, sn, Options{Progress: progress, DryRun: dryRun})

	tempdir := rtest.TempDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := res.RestoreTo(ctx, tempdir)
	rtest.OK(t, err)

	rtest.Equals(t, progressState{
		FilesFinished:   4,
		FilesTotal:      4,
		FilesSkipped:    0,
		AllBytesWritten: 10,
		AllBytesTotal:   10,
		AllBytesSkipped: 0,
	}, progress.state())
}

func TestRestorePermissions(t *testing.T) {
	snapshot := Snapshot{
		Nodes: map[string]Node{
			"foo": File{Data: "content: foo\n", Mode: 0o600, ModTime: time.Now()},
		},
	}

	repo := repository.TestRepository(t)
	tempdir := filepath.Join(rtest.TempDir(t), "target")
	ctx := t.Context()

	sn, id := saveSnapshot(t, repo, snapshot, noopGetGenericAttributes)
	t.Logf("snapshot saved as %v", id.Str())

	res := NewRestorer(repo, sn, Options{})
	_, err := res.RestoreTo(ctx, tempdir)
	rtest.OK(t, err)

	for _, overwrite := range []OverwriteBehavior{OverwriteIfChanged, OverwriteAlways} {
		// tamper with permissions
		path := filepath.Join(tempdir, "foo")
		rtest.OK(t, os.Chmod(path, 0o700))

		res = NewRestorer(repo, sn, Options{Overwrite: overwrite})
		_, err := res.RestoreTo(ctx, tempdir)
		rtest.OK(t, err)
		fi, err := os.Stat(path)
		rtest.OK(t, err)
		rtest.Equals(t, fs.FileMode(0o600), fi.Mode().Perm(), "unexpected permissions")
	}
}

func TestRestorerOverwriteHardlinkedPartial(t *testing.T) {
	baseTime := time.Now().Add(-time.Hour)
	newTime := time.Now()

	parts := []string{"aaaa", "bbbb", "cccc", "dddd"}
	changed := []string{"aaaa", "bbbb", "XXXX", "dddd"}

	baseSnapshot := Snapshot{
		Nodes: map[string]Node{
			"foo": File{DataParts: parts, ModTime: baseTime},
		},
	}
	overwriteSnapshot := Snapshot{
		Nodes: map[string]Node{
			"foo": File{DataParts: changed, ModTime: newTime},
		},
	}

	repo := repository.TestRepository(t)
	tempdir := filepath.Join(rtest.TempDir(t), "target")
	ctx := t.Context()

	sn, _ := saveSnapshot(t, repo, baseSnapshot, noopGetGenericAttributes)
	_, err := NewRestorer(repo, sn, Options{}).RestoreTo(ctx, tempdir)
	rtest.OK(t, err)

	// add a second hard link to the restored file, like `cp -al` does
	target := filepath.Join(tempdir, "foo")
	linked := filepath.Join(rtest.TempDir(t), "linked")
	rtest.OK(t, os.Link(target, linked))

	sn, _ = saveSnapshot(t, repo, overwriteSnapshot, noopGetGenericAttributes)
	for _, overwrite := range []OverwriteBehavior{OverwriteIfChanged, OverwriteAlways} {
		_, err = NewRestorer(repo, sn, Options{Overwrite: overwrite}).RestoreTo(ctx, tempdir)
		rtest.OK(t, err)

		buf, err := os.ReadFile(target)
		rtest.OK(t, err)
		rtest.Equals(t, strings.Join(changed, ""), string(buf))

		// the other hard link must be left untouched
		buf, err = os.ReadFile(linked)
		rtest.OK(t, err)
		rtest.Equals(t, strings.Join(parts, ""), string(buf))

		// restore the old file content and link again for the next round
		rtest.OK(t, os.Remove(target))
		rtest.OK(t, os.Link(linked, target))
	}
}
