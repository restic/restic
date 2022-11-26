//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
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

func testRunMount(t testing.TB, gopts GlobalOptions, dir string, wg *sync.WaitGroup) {
	defer wg.Done()
	opts := MountOptions{
		TimeTemplate: time.RFC3339,
	}
	rtest.OK(t, runMount(context.TODO(), opts, gopts, []string{dir}))
}

func testRunUmount(t testing.TB, gopts GlobalOptions, dir string) {
	var err error
	for i := 0; i < mountWait; i++ {
		if err = umount(dir); err == nil {
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

func checkSnapshots(t testing.TB, global GlobalOptions, repo *repository.Repository, mountpoint, repodir string, snapshotIDs restic.IDs, expectedSnapshotsInFuseDir int) {
	t.Logf("checking for %d snapshots: %v", len(snapshotIDs), snapshotIDs)

	var wg sync.WaitGroup
	wg.Add(1)
	go testRunMount(t, global, mountpoint, &wg)
	waitForMount(t, mountpoint)
	defer wg.Wait()
	defer testRunUmount(t, global, mountpoint)

	if !snapshotsDirExists(t, mountpoint) {
		t.Fatal(`virtual directory "snapshots" doesn't exist`)
	}

	ids := listSnapshots(t, repodir)
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

	for _, id := range snapshotIDs {
		snapshot, err := restic.LoadSnapshot(context.TODO(), repo, id)
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
	env.gopts.backendTestHook = nil
	defer cleanup()

	testRunInit(t, env.gopts)

	repo, err := OpenRepository(context.TODO(), env.gopts)
	rtest.OK(t, err)

	checkSnapshots(t, env.gopts, repo, env.mountpoint, env.repo, []restic.ID{}, 0)

	rtest.SetupTarTestFixture(t, env.testdata, filepath.Join("testdata", "backup-data.tar.gz"))

	// first backup
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	snapshotIDs := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 1,
		"expected one snapshot, got %v", snapshotIDs)

	checkSnapshots(t, env.gopts, repo, env.mountpoint, env.repo, snapshotIDs, 2)

	// second backup, implicit incremental
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	snapshotIDs = testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 2,
		"expected two snapshots, got %v", snapshotIDs)

	checkSnapshots(t, env.gopts, repo, env.mountpoint, env.repo, snapshotIDs, 3)

	// third backup, explicit incremental
	bopts := BackupOptions{Parent: snapshotIDs[0].String()}
	testRunBackup(t, "", []string{env.testdata}, bopts, env.gopts)
	snapshotIDs = testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 3,
		"expected three snapshots, got %v", snapshotIDs)

	checkSnapshots(t, env.gopts, repo, env.mountpoint, env.repo, snapshotIDs, 4)
}

func TestMountSameTimestamps(t *testing.T) {
	if !rtest.RunFuseTest {
		t.Skip("Skipping fuse tests")
	}

	env, cleanup := withTestEnvironment(t)
	// must list snapshots more than once
	env.gopts.backendTestHook = nil
	defer cleanup()

	rtest.SetupTarTestFixture(t, env.base, filepath.Join("testdata", "repo-same-timestamps.tar.gz"))

	repo, err := OpenRepository(context.TODO(), env.gopts)
	rtest.OK(t, err)

	ids := []restic.ID{
		restic.TestParseID("280303689e5027328889a06d718b729e96a1ce6ae9ef8290bff550459ae611ee"),
		restic.TestParseID("75ad6cdc0868e082f2596d5ab8705e9f7d87316f5bf5690385eeff8dbe49d9f5"),
		restic.TestParseID("5fd0d8b2ef0fa5d23e58f1e460188abb0f525c0f0c4af8365a1280c807a80a1b"),
	}

	checkSnapshots(t, env.gopts, repo, env.mountpoint, env.repo, ids, 4)
}
