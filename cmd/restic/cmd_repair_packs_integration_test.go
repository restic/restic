package main

import (
	"bytes"
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

// withCaptureStdoutStderr captures stdout and stderr in a buffer for analysis
func withCaptureStdoutStderr(t testing.TB, gopts global.Options,
	callback func(ctx context.Context, gopts global.Options) error,
) (*bytes.Buffer, *bytes.Buffer, error) {

	bufStdout := bytes.NewBuffer(nil)
	bufStderr := bytes.NewBuffer(nil)
	err := withTermStatusRaw(os.Stdin, bufStdout, bufStderr, gopts, callback)

	return bufStdout, bufStderr, err
}

// testRunRepairPacks runs `restic repair packs` with capturing stdout and stderr
func testRunRepairPacks(t testing.TB, wantJSON bool, gopts global.Options, args []string) ([]byte, []byte, error) {
	bufStdout, bufStderr, err := withCaptureStdoutStderr(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.JSON = wantJSON

		return runRepairPacks(ctx, gopts, gopts.Term, args)
	})

	return bufStdout.Bytes(), bufStderr.Bytes(), err
}

// testRunCheckOutputs runs `restic repair packs` with capturing stderr
func testRunCheckOutputs(t testing.TB, wantJSON bool, gopts global.Options, args []string,
) ([]byte, error) {
	_, bufStderr, err := withCaptureStdoutStderr(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.JSON = wantJSON

		_, err := runCheck(ctx, CheckOptions{}, gopts, args, gopts.Term)
		return err
	})

	return bufStderr.Bytes(), err
}

func TestRunRepairPackfiles(t *testing.T) {
	for _, tpe := range []string{"data", "tree"} {
		t.Run(tpe, func(t *testing.T) {
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
					if blob.Handle().Type.String() == tpe {
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

			outError, err := testRunCheckOutputs(t, false, env.gopts, nil)
			rtest.Assert(t, err != nil, "expected check errors, got none")
			rtest.Assert(t, strings.Contains(string(outError), packIDString), "expected mention of %q", packIDString)

			// restic repair packs 'packIDString'
			_, _, err = testRunRepairPacks(t, false, env.gopts, []string{packIDString})
			rtest.OK(t, err)

			// run restic repair snapshots --forget
			testRunRepairSnapshot(t, env.gopts, true)

			// restic check should produce no errors
			_, err = testRunCheckOutputs(t, false, env.gopts, nil)
			rtest.OK(t, err)
		})
	}
}

func TestWrongPackfile(t *testing.T) {
	// this is the sha2566sum of the zero length file
	wrongPackfile := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)

	_, _, err := withCaptureStdoutStderr(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.JSON = false

		return runRepairPacks(ctx, gopts, gopts.Term, []string{wrongPackfile})
	})

	rtest.Assert(t, err != nil, "expected an error, got none!")
	rtest.Assert(t, strings.Contains(err.Error(), "no ids specified"),
		"expected message `no ids specified`  but got %v", err.Error())
}
