package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
)

func testRunCopy(t testing.TB, srcGopts global.Options, dstGopts global.Options) {
	gopts := srcGopts
	gopts.Repo = dstGopts.Repo
	gopts.Password = dstGopts.Password
	gopts.InsecureNoPassword = dstGopts.InsecureNoPassword
	copyOpts := CopyOptions{
		SecondaryRepoOptions: global.SecondaryRepoOptions{
			Repo:               srcGopts.Repo,
			Password:           srcGopts.Password,
			InsecureNoPassword: srcGopts.InsecureNoPassword,
		},
	}

	rtest.OK(t, withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runCopy(context.TODO(), copyOpts, gopts, nil, gopts.Term)
	}))
}

func TestCopy(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, opts, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "3")}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	testRunInit(t, env2.gopts)
	testRunCopy(t, env.gopts, env2.gopts)

	snapshotIDs := testListSnapshots(t, env.gopts, 3)
	copiedSnapshotIDs := testListSnapshots(t, env2.gopts, 3)

	// Check that the copies size seems reasonable
	stat := dirStats(t, env.repo)
	stat2 := dirStats(t, env2.repo)
	sizeDiff := int64(stat.size) - int64(stat2.size)
	if sizeDiff < 0 {
		sizeDiff = -sizeDiff
	}
	rtest.Assert(t, sizeDiff < int64(stat.size)/50, "expected less than 2%% size difference: %v vs. %v",
		stat.size, stat2.size)

	// Check integrity of the copy
	testRunCheck(t, env2.gopts)

	// Check that the copied snapshots have the same tree contents as the old ones (= identical tree hash)
	origRestores := make(map[string]struct{})
	for i, snapshotID := range snapshotIDs {
		restoredir := filepath.Join(env.base, fmt.Sprintf("restore%d", i))
		origRestores[restoredir] = struct{}{}
		testRunRestore(t, env.gopts, restoredir, snapshotID.String())
	}
	for i, snapshotID := range copiedSnapshotIDs {
		restoredir := filepath.Join(env2.base, fmt.Sprintf("restore%d", i))
		testRunRestore(t, env2.gopts, restoredir, snapshotID.String())
		foundMatch := false
		for cmpdir := range origRestores {
			diff := directoriesContentsDiff(t, restoredir, cmpdir)
			if diff == "" {
				delete(origRestores, cmpdir)
				foundMatch = true
			}
		}

		rtest.Assert(t, foundMatch, "found no counterpart for snapshot %v", snapshotID)
	}

	rtest.Assert(t, len(origRestores) == 0, "found not copied snapshots")

	// check that snapshots were properly batched while copying
	_, _, countBlobs := testPackAndBlobCounts(t, env.gopts)
	countTreePacksDst, countDataPacksDst, countBlobsDst := testPackAndBlobCounts(t, env2.gopts)

	rtest.Equals(t, countBlobs, countBlobsDst, "expected blob count in both repos to be equal")
	rtest.Equals(t, countTreePacksDst, 1, "expected 1 tree packfile")
	rtest.Equals(t, countDataPacksDst, 1, "expected 1 data packfile")
}

func TestCopyPartialSnapshotSelection(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, BackupOptions{}, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "3")}, BackupOptions{}, env.gopts)
	ids := testListSnapshots(t, env.gopts, 2)

	for _, pos := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("missing-at-%d", pos), func(t *testing.T) {
			dst, cleanup := withTestEnvironment(t)
			defer cleanup()
			testRunInit(t, dst.gopts)
			args := []string{ids[0].String(), ids[1].String()}
			args = append(args[:pos], append([]string{"missing-snapshot"}, args[pos:]...)...)
			opts := CopyOptions{SecondaryRepoOptions: global.SecondaryRepoOptions{
				Repo: env.gopts.Repo, Password: env.gopts.Password,
			}}
			err := withTermStatus(t, dst.gopts, func(ctx context.Context, gopts global.Options) error {
				return runCopy(ctx, opts, gopts, args, gopts.Term)
			})
			rtest.Assert(t, err != nil, "copy must report the missing snapshot")
			rtest.Assert(t, strings.Contains(err.Error(), "missing-snapshot"), "error must identify the missing snapshot: %v", err)
			testListSnapshots(t, dst.gopts, 2)
			testRunCheck(t, dst.gopts)
		})
	}
}

type copyLoadFailureBackend struct {
	backend.Backend
	onLoad func(backend.Handle) error
}

func (b *copyLoadFailureBackend) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(io.Reader) error) error {
	if err := b.onLoad(h); err != nil {
		return err
	}
	return b.Backend.Load(ctx, h, length, offset, fn)
}

func TestCopyFatalFailure(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, BackupOptions{}, env.gopts)
	id := testListSnapshots(t, env.gopts, 1)[0].String()

	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprintf("canceled=%t", canceled), func(t *testing.T) {
			dst, cleanup := withTestEnvironment(t)
			defer cleanup()
			testRunInit(t, dst.gopts)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("pack read failed")
			gopts := env.gopts
			gopts.Repo = dst.gopts.Repo
			gopts.Password = dst.gopts.Password
			gopts.NoCache = true
			gopts.BackendTestHook = func(be backend.Backend) (backend.Backend, error) {
				return &copyLoadFailureBackend{Backend: be, onLoad: func(h backend.Handle) error {
					if canceled && h.Type == backend.SnapshotFile {
						cancel()
						return ctx.Err()
					}
					if !canceled && h.Type == backend.PackFile {
						return failure
					}
					return nil
				}}, nil
			}
			opts := CopyOptions{SecondaryRepoOptions: global.SecondaryRepoOptions{
				Repo: env.gopts.Repo, Password: env.gopts.Password,
			}}
			err := withTermStatus(t, gopts, func(_ context.Context, gopts global.Options) error {
				return runCopy(ctx, opts, gopts, []string{"missing-snapshot", id}, gopts.Term)
			})
			rtest.Assert(t, err != nil, "copy must fail")
			if canceled {
				rtest.Assert(t, errors.Is(err, context.Canceled), "must preserve cancellation: %v", err)
			} else {
				rtest.Assert(t, strings.Contains(err.Error(), failure.Error()), "must report data read failure: %v", err)
				rtest.Assert(t, strings.Contains(err.Error(), "missing-snapshot"), "must retain earlier selection failure: %v", err)
			}
			testListSnapshots(t, dst.gopts, 0)
		})
	}
}

func testPackAndBlobCounts(t testing.TB, gopts global.Options) (countTreePacks int, countDataPacks int, countBlobs int) {
	rtest.OK(t, withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		rtest.OK(t, repo.List(context.TODO(), restic.PackFile, func(id restic.ID, size int64) error {
			handles, err := repo.ListPackHandles(context.TODO(), id, size)
			rtest.OK(t, err)
			rtest.Assert(t, len(handles) > 0, "a packfile should contain at least one blob")

			switch handles[0].Type {
			case restic.TreeBlob:
				countTreePacks++
			case restic.DataBlob:
				countDataPacks++
			}
			countBlobs += len(handles)
			return nil
		}))
		return nil
	}))

	return countTreePacks, countDataPacks, countBlobs
}

func TestCopyIncremental(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	testRunInit(t, env2.gopts)
	testRunCopy(t, env.gopts, env2.gopts)

	testListSnapshots(t, env.gopts, 2)
	testListSnapshots(t, env2.gopts, 2)

	// Check that the copies size seems reasonable
	testRunCheck(t, env2.gopts)

	// check that no snapshots are copied, as there are no new ones
	testRunCopy(t, env.gopts, env2.gopts)
	testRunCheck(t, env2.gopts)
	testListSnapshots(t, env2.gopts, 2)

	// check that only new snapshots are copied
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "3")}, opts, env.gopts)
	testRunCopy(t, env.gopts, env2.gopts)
	testRunCheck(t, env2.gopts)
	testListSnapshots(t, env.gopts, 3)
	testListSnapshots(t, env2.gopts, 3)

	// also test the reverse direction
	testRunCopy(t, env2.gopts, env.gopts)
	testRunCheck(t, env.gopts)
	testListSnapshots(t, env.gopts, 3)
}

func TestCopyUnstableJSON(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()

	// contains a symlink created using `ln -s '../i/'$'\355\246\361''d/samba' broken-symlink`
	datafile := filepath.Join("testdata", "copy-unstable-json.tar.gz")
	rtest.SetupTarTestFixture(t, env.base, datafile)

	testRunInit(t, env2.gopts)
	testRunCopy(t, env.gopts, env2.gopts)
	testRunCheck(t, env2.gopts)
	testListSnapshots(t, env2.gopts, 1)
}

func TestCopyToEmptyPassword(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()
	env2.gopts.Password = ""
	env2.gopts.InsecureNoPassword = true

	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, BackupOptions{}, env.gopts)

	testRunInit(t, env2.gopts)
	testRunCopy(t, env.gopts, env2.gopts)

	testListSnapshots(t, env.gopts, 1)
	testListSnapshots(t, env2.gopts, 1)
	testRunCheck(t, env2.gopts)
}
