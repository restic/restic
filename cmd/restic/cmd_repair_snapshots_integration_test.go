package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"hash/fnv"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunRepairSnapshot(t testing.TB, gopts global.Options, forget bool) {
	opts := RepairOptions{
		Forget: forget,
	}

	rtest.OK(t, withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runRepairSnapshots(context.TODO(), gopts, opts, nil, gopts.Term)
	}))
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
	_, _, err := testRunCheckOutput(t, env.gopts, false)
	rtest.OK(t, err)
}

func TestRepairSnapshotsWithLostTree(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	createRandomFile(t, env, "foo/bar/file", 12345)
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	oldSnapshot := testListSnapshots(t, env.gopts, 1)
	oldPacks := testRunList(t, env.gopts, "packs")

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
	_, _, err := testRunCheckOutput(t, env.gopts, false)
	rtest.OK(t, err)
}

func TestRepairSnapshotsWithLostRootTree(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	createRandomFile(t, env, "foo/bar/file", 12345)
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)
	oldPacks := testRunList(t, env.gopts, "packs")

	// remove all trees
	removePacks(env.gopts, t, restic.NewIDSet(oldPacks...))
	testRunCheckMustFail(t, env.gopts)

	// repair
	testRunRebuildIndex(t, env.gopts)
	testRunRepairSnapshot(t, env.gopts, true)
	testListSnapshots(t, env.gopts, 0)
	_, _, err := testRunCheckOutput(t, env.gopts, false)
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

func TestRepairSnapshotsBrokenSnapshots(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	// create backup
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, BackupOptions{}, env.gopts)

	// create zero length file in snapshots/
	// will fail with
	// failed to load snapshot 1d204771: LoadRaw(<snapshot/1d20477115>): invalid data returned
	handle, err := os.Create(filepath.Join(env.repo, "snapshots", "1d20477115fb872069a28a80ffb95a82cb8b1b1920de046a68c0195da63f30cf"))
	rtest.OK(t, err)
	rtest.OK(t, handle.Close())

	// create some file with a correct sha256 name in snapshots/, will fail with
	// failed to load snapshot abcd1234: ciphertext verification failed
	contents := rtest.Random(1234567890123, 42)
	sha256Contents := sha256.Sum256(contents)
	target := hex.EncodeToString(sha256Contents[:])
	rtest.OK(t, os.WriteFile(filepath.Join(env.repo, "snapshots", target), contents, 0o600))

	// run repair snapshots
	repairOpts := RepairOptions{Forget: true}
	env.gopts.BackendTestHook = nil
	_, err = withCaptureStdout(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runRepairSnapshots(ctx, gopts, repairOpts, []string{"1d204771", target[:8]}, gopts.Term)
	})
	rtest.OK(t, err)

	// verify that there are no snapshot errors
	testRunCheck(t, env.gopts)
}

func TestCheckWithDamagedSnaphotFile(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, env.testdata+"/0", []string{"for_cmd_ls"}, opts, env.gopts)
	snID := testListSnapshots(t, env.gopts, 1)[0]

	// modify permissions for snapshot file to "no permissions"
	handle, err := os.Open(filepath.Join(env.gopts.Repo, "snapshots", snID.String()))
	rtest.OK(t, err)
	rtest.OK(t, handle.Chmod(0o000))
	rtest.OK(t, handle.Close())

	// run check - which must fail
	_, stderr, err := testRunCheckOutput(t, env.gopts, false)
	rtest.Assert(t, err != nil, "expected non nil error after check of damaged repository")
	rtest.Assert(t, strings.Contains(stderr, "permission denied"), "expected permission error")

	repairOpts := RepairOptions{Forget: true}
	_, err = withCaptureStdout(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		// run restic --no-cache repair snapshots --forget <ID>
		// disabling cache is essential for this test to succeed
		gopts.NoCache = true
		gopts.BackendTestHook = nil

		return runRepairSnapshots(ctx, gopts, repairOpts, []string{snID.String()}, gopts.Term)
	})
	rtest.OK(t, err)

	// verify that snapshot has been removed
	testListSnapshots(t, env.gopts, 0)
}
