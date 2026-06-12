//go:build darwin || freebsd || linux

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	systemFuse "github.com/anacrolix/fuse"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui"
)

const (
	mountWait       = 20
	mountSleep      = 100 * time.Millisecond
	mountTestSubdir = "snapshots"
)

func snapshotsDirExists(t testing.TB, dir string) bool {
	f, err := os.Open(filepath.Join(dir, mountTestSubdir))
	if err != nil && os.IsNotExist(err) {
		return false
	}

	if err != nil {
		t.Error(err)
	}

	if err := f.Close(); err != nil {
		t.Error(err)
	}

	return true
}

// waitForMount blocks (max mountWait * mountSleep) until the subdir
// "snapshots" appears in the dir.
func waitForMount(t testing.TB, dir string) {
	for i := 0; i < mountWait; i++ {
		if snapshotsDirExists(t, dir) {
			t.Log("mounted directory is ready")
			return
		}

		time.Sleep(mountSleep)
	}

	t.Errorf("subdir %q of dir %s never appeared", mountTestSubdir, dir)
}

func testRunMount(t testing.TB, gopts global.Options, dir string, wg *sync.WaitGroup) {
	defer wg.Done()
	opts := MountOptions{
		TimeTemplate: time.RFC3339,
	}
	rtest.OK(t, withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runMount(context.TODO(), opts, gopts, []string{dir}, gopts.Term)
	}))
}

func testRunUmount(t testing.TB, dir string) {
	var err error
	for i := 0; i < mountWait; i++ {
		if err = systemFuse.Unmount(dir); err == nil {
			t.Logf("directory %v umounted", dir)
			return
		}

		time.Sleep(mountSleep)
	}

	t.Errorf("unable to umount dir %v, last error was: %v", dir, err)
}

func listSnapshots(t testing.TB, dir string) []string {
	snapshotsDir, err := os.Open(filepath.Join(dir, "snapshots"))
	rtest.OK(t, err)
	names, err := snapshotsDir.Readdirnames(-1)
	rtest.OK(t, err)
	rtest.OK(t, snapshotsDir.Close())
	return names
}

func checkSnapshots(t testing.TB, gopts global.Options, mountpoint string, snapshotIDs restic.IDs, expectedSnapshotsInFuseDir int) {
	t.Logf("checking for %d snapshots: %v", len(snapshotIDs), snapshotIDs)

	var wg sync.WaitGroup
	wg.Add(1)
	go testRunMount(t, gopts, mountpoint, &wg)
	waitForMount(t, mountpoint)
	defer wg.Wait()
	defer testRunUmount(t, mountpoint)

	if !snapshotsDirExists(t, mountpoint) {
		t.Fatal(`virtual directory "snapshots" doesn't exist`)
	}

	ids := listSnapshots(t, gopts.Repo)
	t.Logf("found %v snapshots in repo: %v", len(ids), ids)

	namesInSnapshots := listSnapshots(t, mountpoint)
	t.Logf("found %v snapshots in fuse mount: %v", len(namesInSnapshots), namesInSnapshots)
	rtest.Assert(t,
		expectedSnapshotsInFuseDir == len(namesInSnapshots),
		"Invalid number of snapshots: expected %d, got %d", expectedSnapshotsInFuseDir, len(namesInSnapshots))

	namesMap := make(map[string]bool)
	for _, name := range namesInSnapshots {
		namesMap[name] = false
	}

	// Is "latest" present?
	if len(namesMap) != 0 {
		_, ok := namesMap["latest"]
		if !ok {
			t.Errorf("Symlink latest isn't present in fuse dir")
		} else {
			namesMap["latest"] = true
		}
	}

	err := withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		if err != nil {
			return err
		}
		defer unlock()

		for _, id := range snapshotIDs {
			snapshot, err := data.LoadSnapshot(ctx, repo, id)
			rtest.OK(t, err)

			ts := snapshot.Time.Format(time.RFC3339)
			present, ok := namesMap[ts]
			if !ok {
				t.Errorf("Snapshot %v (%q) isn't present in fuse dir", id.Str(), ts)
			}

			for i := 1; present; i++ {
				ts = fmt.Sprintf("%s-%d", snapshot.Time.Format(time.RFC3339), i)
				present, ok = namesMap[ts]
				if !ok {
					t.Errorf("Snapshot %v (%q) isn't present in fuse dir", id.Str(), ts)
				}

				if !present {
					break
				}
			}

			namesMap[ts] = true
		}
		return nil
	})
	rtest.OK(t, err)

	for name, present := range namesMap {
		rtest.Assert(t, present, "Directory %s is present in fuse dir but is not a snapshot", name)
	}
}

func TestMount(t *testing.T) {
	if !rtest.RunFuseTest {
		t.Skip("Skipping fuse tests")
	}

	env, cleanup := withTestEnvironment(t)
	// must list snapshots more than once
	env.gopts.BackendTestHook = nil
	defer cleanup()

	testRunInit(t, env.gopts)

	checkSnapshots(t, env.gopts, env.mountpoint, []restic.ID{}, 0)

	rtest.SetupTarTestFixture(t, env.testdata, filepath.Join("testdata", "backup-data.tar.gz"))

	// first backup
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	snapshotIDs := testRunList(t, env.gopts, "snapshots")
	rtest.Assert(t, len(snapshotIDs) == 1,
		"expected one snapshot, got %v", snapshotIDs)

	checkSnapshots(t, env.gopts, env.mountpoint, snapshotIDs, 2)

	// second backup, implicit incremental
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	snapshotIDs = testRunList(t, env.gopts, "snapshots")
	rtest.Assert(t, len(snapshotIDs) == 2,
		"expected two snapshots, got %v", snapshotIDs)

	checkSnapshots(t, env.gopts, env.mountpoint, snapshotIDs, 3)

	// third backup, explicit incremental
	bopts := BackupOptions{Parent: snapshotIDs[0].String()}
	testRunBackup(t, "", []string{env.testdata}, bopts, env.gopts)
	snapshotIDs = testRunList(t, env.gopts, "snapshots")
	rtest.Assert(t, len(snapshotIDs) == 3,
		"expected three snapshots, got %v", snapshotIDs)

	checkSnapshots(t, env.gopts, env.mountpoint, snapshotIDs, 4)
}

func TestCheckMountpointOverlap(t *testing.T) {
	tempdir := t.TempDir()
	repo := filepath.Join(tempdir, "repo")
	repoSub := filepath.Join(repo, "sub")
	sibling := filepath.Join(tempdir, "mnt")
	for _, d := range []string{repo, repoSub, sibling} {
		rtest.OK(t, os.MkdirAll(d, 0700))
	}

	cases := []struct {
		name    string
		repo    string
		mount   string
		wantSub string // substring of expected error; empty means expect nil
	}{
		{"equal", repo, repo, "is the local repository directory"},
		{"mount inside repo", repo, repoSub, "is inside the local repository directory"},
		{"repo inside mount", repoSub, repo, "is inside the mountpoint"},
		{"disjoint", repo, sibling, ""},
		{"prefix-not-subpath", repo, repo + "-other", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rtest.OK(t, os.MkdirAll(tc.mount, 0700))
			err := checkMountpointOverlap(tc.repo, tc.mount)
			if tc.wantSub == "" {
				rtest.OK(t, err)
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestCheckMountpointOverlapSymlink(t *testing.T) {
	tempdir := t.TempDir()
	repo := filepath.Join(tempdir, "repo")
	rtest.OK(t, os.MkdirAll(repo, 0700))
	link := filepath.Join(tempdir, "link-to-repo")
	rtest.OK(t, os.Symlink(repo, link))

	err := checkMountpointOverlap(repo, link)
	if err == nil {
		t.Fatal("expected overlap error when mountpoint is a symlink to repo, got nil")
	}
	if !strings.Contains(err.Error(), "is the local repository directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMountSameTimestamps(t *testing.T) {
	if !rtest.RunFuseTest {
		t.Skip("Skipping fuse tests")
	}

	debugEnabled := debug.TestLogToStderr(t)
	if debugEnabled {
		defer debug.TestDisableLog(t)
	}

	env, cleanup := withTestEnvironment(t)
	// must list snapshots more than once
	env.gopts.BackendTestHook = nil
	defer cleanup()

	rtest.SetupTarTestFixture(t, env.base, filepath.Join("testdata", "repo-same-timestamps.tar.gz"))

	ids := []restic.ID{
		restic.TestParseID("280303689e5027328889a06d718b729e96a1ce6ae9ef8290bff550459ae611ee"),
		restic.TestParseID("75ad6cdc0868e082f2596d5ab8705e9f7d87316f5bf5690385eeff8dbe49d9f5"),
		restic.TestParseID("5fd0d8b2ef0fa5d23e58f1e460188abb0f525c0f0c4af8365a1280c807a80a1b"),
	}

	checkSnapshots(t, env.gopts, env.mountpoint, ids, 4)
}
