// +build !openbsd

package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

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
		stSnapshots, err := os.Open(filepath.Join(mountpoint, "snapshots"))
		OK(t, err)
		namesInSnapshots, err := stSnapshots.Readdirnames(-1)
		OK(t, err)
		Assert(t,
			len(namesInSnapshots) == len(snapshotIDs),
			"Invalid number of snapshots: expected %d, got %d", len(snapshotIDs), len(namesInSnapshots))

		for i, id := range snapshotIDs {
			snapshot, err := restic.LoadSnapshot(repo, id)
			OK(t, err)
			Assert(t,
				namesInSnapshots[i] == snapshot.Time.Format(time.RFC3339),
				"Invalid snapshot directory name: expected %s, got %s", snapshot.Time.Format(time.RFC3339), namesInSnapshots[i])
		}
		OK(t, stSnapshots.Close())
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

		stMountPoint, err := os.Open(mountpoint)
		OK(t, err)
		names, err := stMountPoint.Readdirnames(-1)
		OK(t, err)
		Assert(t, len(names) == 1 && names[0] == "snapshots", "expected the snapshots directory to exist")
		OK(t, stMountPoint.Close())

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
