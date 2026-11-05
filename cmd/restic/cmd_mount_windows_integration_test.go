//go:build windows

package main

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/winfsp/go-winfsp"

	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

const (
	mountWait  = 400
	mountSleep = 5 * time.Millisecond
)

// waitForMount blocks (max mountWait * mountSleep) until the subdir
// "snapshots" appears in the dir.
func waitForMount(t testing.TB, dir string) {
	for range mountWait {
		if _, err := os.Stat(filepath.Join(dir, "snapshots")); err == nil {
			t.Log("mounted directory is ready")
			return
		}

		time.Sleep(mountSleep)
	}

	t.Fatalf(`subdir "snapshots" of dir %s never appeared`, dir)
}

func listNames(t testing.TB, dir string) []string {
	entries, err := os.ReadDir(dir)
	rtest.OK(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestMount(t *testing.T) {
	if !rtest.RunFuseTest {
		t.Skip("Skipping fuse tests")
	}
	if _, err := winfsp.BinPath(); err != nil {
		t.Skipf("WinFSP is not installed: %v", err)
	}

	env, cleanup := withTestEnvironment(t)
	// must list snapshots more than once
	env.gopts.BackendTestHook = nil
	defer cleanup()

	testRunInit(t, env.gopts)
	rtest.SetupTarTestFixture(t, env.testdata, filepath.Join("testdata", "backup-data.tar.gz"))
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	snapshotIDs := testRunList(t, env.gopts, "snapshots")
	rtest.Assert(t, len(snapshotIDs) == 1, "expected one snapshot, got %v", snapshotIDs)

	// WinFSP requires the mountpoint to not exist yet
	mountpoint := filepath.Join(env.base, "winfsp")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- withTermStatus(t, env.gopts, func(_ context.Context, gopts global.Options) error {
			opts := MountOptions{TimeTemplate: defaultTimeTemplate}
			return runMount(ctx, opts, gopts, []string{mountpoint}, gopts.Term)
		})
	}()
	defer func() {
		cancel()
		err := <-done
		rtest.Assert(t, errors.Is(err, ErrOK), "unexpected error from mount: %v", err)
	}()
	waitForMount(t, mountpoint)

	rtest.Equals(t, []string{"hosts", "ids", "snapshots", "tags"}, listNames(t, mountpoint))
	rtest.Equals(t, []string{snapshotIDs[0].Str()}, listNames(t, filepath.Join(mountpoint, "ids")))
	names := listNames(t, filepath.Join(mountpoint, "snapshots"))
	rtest.Assert(t, len(names) == 2 && names[1] == "latest", "unexpected snapshots directory content: %v", names)

	// compare some files with the backed up data
	snapshotDir := filepath.Join(mountpoint, "ids", snapshotIDs[0].Str())
	var mounted []string
	rtest.OK(t, filepath.WalkDir(snapshotDir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			mounted = append(mounted, path)
		}
		return err
	}))

	checked := 0
	rtest.OK(t, filepath.WalkDir(env.testdata, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if checked >= 5 {
			return filepath.SkipAll
		}

		rel, err := filepath.Rel(env.testdata, path)
		rtest.OK(t, err)
		var match string
		for _, m := range mounted {
			if strings.HasSuffix(m, string(filepath.Separator)+rel) {
				match = m
				break
			}
		}
		rtest.Assert(t, match != "", "file %v not found in mount", rel)

		expected, err := os.ReadFile(path)
		rtest.OK(t, err)
		actual, err := os.ReadFile(match)
		rtest.OK(t, err)
		rtest.Assert(t, bytes.Equal(expected, actual), "content of %v differs", rel)
		checked++
		return nil
	}))
	rtest.Assert(t, checked > 0, "no files compared")

	// the file system is read-only
	err := os.WriteFile(filepath.Join(mountpoint, "foo"), []byte("bar"), 0644)
	rtest.Assert(t, err != nil, "expected error when writing to the mount")
}
