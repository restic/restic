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
	"github.com/restic/restic/internal/ui"
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

// testRunRepairPacks runs `restic repair packs` with capturing std and stderr
func testRunRepairPacks(t testing.TB, wantJSON bool, gopts global.Options, args []string) ([]byte, []byte, error) {
	bufStdout, bufStderr, err := withCaptureStdoutStderr(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.JSON = wantJSON
		gopts.Quiet = true

		return runRepairPacks(ctx, gopts, gopts.Term, args)
	})

	return bufStdout.Bytes(), bufStderr.Bytes(), err
}

// testRunCheckOutputs runs `restic repair packs` with capturing std and stderr
func testRunCheckOutputs(t testing.TB, wantJSON bool, gopts global.Options, args []string) ([]byte, []byte, error) {
	var errInner error
	bufStdout, bufStderr, err := withCaptureStdoutStderr(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.JSON = wantJSON
		gopts.Quiet = true

		_, errInner = runCheck(ctx, CheckOptions{}, gopts, args, gopts.Term)
		return errInner
	})

	return bufStdout.Bytes(), bufStderr.Bytes(), err
}

func TestRunRepairPackfiles(t *testing.T) {
	for _, name := range []string{"data", "tree"} {
		t.Run(name, func(t *testing.T) {
			env, cleanup := withTestEnvironment(t)
			defer cleanup()

			testSetupBackupData(t, env)
			// backup of subtree 0/0/9/42
			testRunBackup(t, env.testdata, []string{filepath.Join(env.testdata, "0", "0", "9", "42")}, BackupOptions{}, env.gopts)
			testListSnapshots(t, env.gopts, 1)

			packfileID := restic.ID{}
			err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
				printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
				_, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
				rtest.OK(t, err)
				defer unlock()

				rtest.OK(t, repo.LoadIndex(ctx, printer))
				err = repo.ListBlobs(ctx, func(blob restic.PackedBlob) {
					if blob.Type.String() == name {
						packfileID = blob.PackID
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
			t.Logf("remove data packfile %q", filename)
			rtest.OK(t, os.Remove(filename))

			_, outError, err := testRunCheckOutputs(t, false, env.gopts, nil)
			rtest.Assert(t, err != nil, "expected check errors")
			rtest.Assert(t, strings.Contains(string(outError), packIDString), "expected mention of %q", packIDString)

			// repair pack
			out, outErr, err := testRunRepairPacks(t, false, env.gopts, []string{packIDString})
			rtest.Assert(t, len(out) == 0, "expected no normal output, got %v", out)
			t.Logf("errs\n%s", string(outErr))
			rtest.OK(t, err)

			// run restic repair snapshots --forget
			testRunRepairSnapshot(t, env.gopts, true)
			_, _, err = testRunCheckOutputs(t, false, env.gopts, nil)
			rtest.OK(t, err)
		})
	}
}
