package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
)

// testRunRepairPacks runs `restic repair packs` with capturing stdout and stderr
func testRunRepairPacks(t testing.TB, gopts global.Options, args []string) (string, string, error) {
	bufStdout, bufStderr, err := withCaptureStdoutStderr(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runRepairPacks(ctx, gopts, gopts.Term, args)
	})

	return bufStdout.String(), bufStderr.String(), err
}

func TestRunRepairPackfiles(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	// backup of subtree 0/0/9/42
	testRunBackup(t, env.testdata, []string{filepath.Join(env.testdata, "0", "0", "9", "42")}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	packfileID := restic.ID{}
	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := progress.NewTerminalPrinter(false, gopts.Verbosity, gopts.Term)
		_, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		rtest.OK(t, repo.LoadIndex(ctx, printer))
		// load packfiles from master index
		err = repo.ListBlobs(ctx, func(blob restic.PackBlob) {
			if blob.Handle().Type == restic.DataBlob {
				packfileID = blob.PackID()
				return
			}
		})
		rtest.OK(t, err)

		return nil
	})
	rtest.OK(t, err)

	rtest.Assert(t, !packfileID.IsNull(), "expected valid packfile ID")
	packIDString := packfileID.String()
	filename := filepath.Join(env.gopts.Repo, "data", packIDString[0:2], packIDString)
	rtest.OK(t, os.Remove(filename))

	outError, err := testRunCheckErrorOutput(t, env.gopts)
	rtest.Assert(t, err != nil, "expected check errors, got none")
	rtest.Assert(t, strings.Contains(string(outError), packIDString), "expected mention of %q", packIDString)

	// change to temporary directory to not pollute the repository with backup files
	cleanupChdir := rtest.Chdir(t, env.base)
	defer cleanupChdir()
	// restic repair packs 'packIDString'
	_, _, err = testRunRepairPacks(t, env.gopts, []string{packIDString})
	rtest.OK(t, err)

	// run restic repair snapshots --forget
	testRunRepairSnapshot(t, env.gopts, true)

	// restic check should produce no errors
	_, err = testRunCheckErrorOutput(t, env.gopts)
	rtest.OK(t, err)
}
