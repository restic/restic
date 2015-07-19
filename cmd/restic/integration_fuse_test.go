// +build !openbsd

package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bazil.org/fuse"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/repository"
	. "github.com/restic/restic/test"
)

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
		OK(t, os.RemoveAll(mountpoint))

		ready := make(chan struct{}, 1)
		go cmdMount(t, global, mountpoint, ready)
		<-ready

		defer func() {
			err := fuse.Unmount(mountpoint)
			OK(t, err)
			if TestCleanup {
				err = os.RemoveAll(mountpoint)
				OK(t, err)
			}
		}()

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
		cmdBackup(t, global, []string{env.testdata}, snapshotIDs[0])
		snapshotIDs = cmdList(t, global, "snapshots")
		Assert(t, len(snapshotIDs) == 3,
			"expected three snapshots, got %v", snapshotIDs)

		checkSnapshots(repo, mountpoint, snapshotIDs)
	})
}
