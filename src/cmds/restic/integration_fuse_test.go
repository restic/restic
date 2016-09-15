// +build !openbsd
// +build !windows

package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"restic"
	"restic/repository"
	. "restic/test"
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

func mount(t testing.TB, global GlobalOptions, dir string) {
	cmd := &CmdMount{global: &global}
	OK(t, cmd.Mount(dir))
}

func umount(t testing.TB, global GlobalOptions, dir string) {
	cmd := &CmdMount{global: &global}

	var err error
	for i := 0; i < mountWait; i++ {
		if err = cmd.Umount(dir); err == nil {
			t.Logf("directory %v umounted", dir)
			return
		}

		time.Sleep(mountSleep)
	}

	t.Errorf("unable to umount dir %v, last error was: %v", dir, err)
}

func listSnapshots(t testing.TB, dir string) []string {
	snapshotsDir, err := os.Open(filepath.Join(dir, "snapshots"))
	OK(t, err)
	names, err := snapshotsDir.Readdirnames(-1)
	OK(t, err)
	OK(t, snapshotsDir.Close())
	return names
}

func TestMount(t *testing.T) {
	if !RunFuseTest {
		t.Skip("Skipping fuse tests")
	}

	withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
		checkSnapshots := func(repo *repository.Repository, mountpoint string, snapshotIDs restic.IDs) {
			t.Logf("checking for %d snapshots: %v", len(snapshotIDs), snapshotIDs)
			go mount(t, global, mountpoint)
			waitForMount(t, mountpoint)
			defer umount(t, global, mountpoint)

			if !snapshotsDirExists(t, mountpoint) {
				t.Fatal(`virtual directory "snapshots" doesn't exist`)
			}

			ids := listSnapshots(t, env.repo)
			t.Logf("found %v snapshots in repo: %v", len(ids), ids)

			namesInSnapshots := listSnapshots(t, mountpoint)
			t.Logf("found %v snapshots in fuse mount: %v", len(namesInSnapshots), namesInSnapshots)
			Assert(t,
				len(namesInSnapshots) == len(snapshotIDs),
				"Invalid number of snapshots: expected %d, got %d", len(snapshotIDs), len(namesInSnapshots))

			namesMap := make(map[string]bool)
			for _, name := range namesInSnapshots {
				namesMap[name] = false
			}

			for _, id := range snapshotIDs {
				snapshot, err := restic.LoadSnapshot(repo, id)
				OK(t, err)
				_, ok := namesMap[snapshot.Time.Format(time.RFC3339)]
				Assert(t, ok, "Snapshot %s isn't present in fuse dir", snapshot.Time.Format(time.RFC3339))
				namesMap[snapshot.Time.Format(time.RFC3339)] = true
			}
			for name, present := range namesMap {
				Assert(t, present, "Directory %s is present in fuse dir but is not a snapshot", name)
			}
		}

		cmdInit(t, global)
		repo, err := global.OpenRepository()
		OK(t, err)

		mountpoint, err := ioutil.TempDir(TestTempDir, "restic-test-mount-")
		OK(t, err)

		// We remove the mountpoint now to check that cmdMount creates it
		RemoveAll(t, mountpoint)

		checkSnapshots(repo, mountpoint, []restic.ID{})

		SetupTarTestFixture(t, env.testdata, filepath.Join("testdata", "backup-data.tar.gz"))

		// first backup
		cmdBackup(t, global, []string{env.testdata}, nil)
		snapshotIDs := cmdList(t, global, "snapshots")
		Assert(t, len(snapshotIDs) == 1,
			"expected one snapshot, got %v", snapshotIDs)

		checkSnapshots(repo, mountpoint, snapshotIDs)

		// second backup, implicit incremental
		cmdBackup(t, global, []string{env.testdata}, nil)
		snapshotIDs = cmdList(t, global, "snapshots")
		Assert(t, len(snapshotIDs) == 2,
			"expected two snapshots, got %v", snapshotIDs)

		checkSnapshots(repo, mountpoint, snapshotIDs)

		// third backup, explicit incremental
		cmdBackup(t, global, []string{env.testdata}, &snapshotIDs[0])
		snapshotIDs = cmdList(t, global, "snapshots")
		Assert(t, len(snapshotIDs) == 3,
			"expected three snapshots, got %v", snapshotIDs)

		checkSnapshots(repo, mountpoint, snapshotIDs)
	})
}
