// +build !openbsd
// +build !windows

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"restic"
	"restic/backend"
	"restic/repository"
	. "restic/test"
)

const (
	mountWait       = 20
	mountSleep      = 100 * time.Millisecond
	mountTestSubdir = "snapshots"
)

// waitForMount blocks (max mountWait * mountSleep) until the subdir
// "snapshots" appears in the dir.
func waitForMount(dir string) error {
	for i := 0; i < mountWait; i++ {
		f, err := os.Open(dir)
		if err != nil {
			return err
		}

		names, err := f.Readdirnames(-1)
		if err != nil {
			return err
		}

		if err = f.Close(); err != nil {
			return err
		}

		for _, name := range names {
			if name == mountTestSubdir {
				return nil
			}
		}

		time.Sleep(mountSleep)
	}

	return fmt.Errorf("subdir %q of dir %s never appeared", mountTestSubdir, dir)
}

func cmdMount(t testing.TB, global GlobalOptions, dir string, ready, done chan struct{}) {
	defer func() {
		ready <- struct{}{}
	}()

	cmd := &CmdMount{global: &global, ready: ready, done: done}
	OK(t, cmd.Execute([]string{dir}))
	if TestCleanupTempDirs {
		RemoveAll(t, dir)
	}
}

func TestMount(t *testing.T) {
	if !RunFuseTest {
		t.Skip("Skipping fuse tests")
	}

	checkSnapshots := func(repo *repository.Repository, mountpoint string, snapshotIDs []backend.ID) {
		snapshotsDir, err := os.Open(filepath.Join(mountpoint, "snapshots"))
		OK(t, err)
		namesInSnapshots, err := snapshotsDir.Readdirnames(-1)
		OK(t, err)
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
		OK(t, snapshotsDir.Close())
	}

	withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
		cmdInit(t, global)
		repo, err := global.OpenRepository()
		OK(t, err)

		mountpoint, err := ioutil.TempDir(TestTempDir, "restic-test-mount-")
		OK(t, err)

		// We remove the mountpoint now to check that cmdMount creates it
		RemoveAll(t, mountpoint)

		ready := make(chan struct{}, 2)
		done := make(chan struct{})
		go cmdMount(t, global, mountpoint, ready, done)
		<-ready
		defer close(done)
		OK(t, waitForMount(mountpoint))

		mountpointDir, err := os.Open(mountpoint)
		OK(t, err)
		names, err := mountpointDir.Readdirnames(-1)
		OK(t, err)
		Assert(t, len(names) == 1 && names[0] == "snapshots", `The fuse virtual directory "snapshots" doesn't exist`)
		OK(t, mountpointDir.Close())

		checkSnapshots(repo, mountpoint, []backend.ID{})

		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(err) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		SetupTarTestFixture(t, env.testdata, datafile)

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
