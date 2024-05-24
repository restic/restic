package main

import (
	"context"
	"hash/fnv"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunRepairSnapshot(t testing.TB, gopts GlobalOptions, forget bool) {
	opts := RepairOptions{
		Forget: forget,
	}

	rtest.OK(t, runRepairSnapshots(context.TODO(), gopts, opts, nil))
}

func createRandomFile(t testing.TB, env *testEnvironment, path string, size int) {
	fn := filepath.Join(env.testdata, path)
	rtest.OK(t, os.MkdirAll(filepath.Dir(fn), 0o755))

	h := fnv.New64()
	_, err := h.Write([]byte(path))
	rtest.OK(t, err)
	r := rand.New(rand.NewSource(int64(h.Sum64())))

	f, err := os.OpenFile(fn, os.O_CREATE|os.O_RDWR, 0o644)
	rtest.OK(t, err)
	_, err = io.Copy(f, io.LimitReader(r, int64(size)))
	rtest.OK(t, err)
	rtest.OK(t, f.Close())
}

func TestRepairSnapshotsWithLostData(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	createRandomFile(t, env, "foo/bar/file", 512*1024)
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)
	// damage repository
	removePacksExcept(env.gopts, t, restic.NewIDSet(), false)

	createRandomFile(t, env, "foo/bar/file2", 256*1024)
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	snapshotIDs := testListSnapshots(t, env.gopts, 2)
	testRunCheckMustFail(t, env.gopts)

	// repair but keep broken snapshots
	testRunRebuildIndex(t, env.gopts)
	testRunRepairSnapshot(t, env.gopts, false)
	testListSnapshots(t, env.gopts, 4)
	testRunCheckMustFail(t, env.gopts)

	// repository must be ok after removing the broken snapshots
	testRunForget(t, env.gopts, ForgetOptions{}, snapshotIDs[0].String(), snapshotIDs[1].String())
	testListSnapshots(t, env.gopts, 2)
	_, err := testRunCheckOutput(env.gopts, false)
	rtest.OK(t, err)
}

func TestRepairSnapshotsWithLostTree(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	createRandomFile(t, env, "foo/bar/file", 12345)
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	oldSnapshot := testListSnapshots(t, env.gopts, 1)
	oldPacks := testRunList(t, "packs", env.gopts)

	// keep foo/bar unchanged
	createRandomFile(t, env, "foo/bar2", 1024)
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 2)

	// remove tree for foo/bar and the now completely broken first snapshot
	removePacks(env.gopts, t, restic.NewIDSet(oldPacks...))
	testRunForget(t, env.gopts, ForgetOptions{}, oldSnapshot[0].String())
	testRunCheckMustFail(t, env.gopts)

	// repair
	testRunRebuildIndex(t, env.gopts)
	testRunRepairSnapshot(t, env.gopts, true)
	testListSnapshots(t, env.gopts, 1)
	_, err := testRunCheckOutput(env.gopts, false)
	rtest.OK(t, err)
}

func TestRepairSnapshotsWithLostRootTree(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	createRandomFile(t, env, "foo/bar/file", 12345)
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)
	oldPacks := testRunList(t, "packs", env.gopts)

	// remove all trees
	removePacks(env.gopts, t, restic.NewIDSet(oldPacks...))
	testRunCheckMustFail(t, env.gopts)

	// repair
	testRunRebuildIndex(t, env.gopts)
	testRunRepairSnapshot(t, env.gopts, true)
	testListSnapshots(t, env.gopts, 0)
	_, err := testRunCheckOutput(env.gopts, false)
	rtest.OK(t, err)
}

func TestRepairSnapshotsIntact(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testSetupBackupData(t, env)
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, BackupOptions{}, env.gopts)
	oldSnapshotIDs := testListSnapshots(t, env.gopts, 1)

	// use an exclude that will not exclude anything
	testRunRepairSnapshot(t, env.gopts, false)
	snapshotIDs := testListSnapshots(t, env.gopts, 1)
	rtest.Assert(t, reflect.DeepEqual(oldSnapshotIDs, snapshotIDs), "unexpected snapshot id mismatch %v vs. %v", oldSnapshotIDs, snapshotIDs)
	testRunCheck(t, env.gopts)
}
