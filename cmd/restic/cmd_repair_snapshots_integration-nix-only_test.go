//go:build !windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/internal/global"
	rtest "github.com/restic/restic/internal/test"
)

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
