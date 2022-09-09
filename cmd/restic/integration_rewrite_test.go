package main

import (
	"context"
	"path/filepath"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func testRunRewriteExclude(t testing.TB, gopts GlobalOptions, excludes []string) {
	opts := RewriteOptions{
		excludePatternOptions: excludePatternOptions{
			Excludes: excludes,
		},
	}

	rtest.OK(t, runRewrite(context.TODO(), opts, gopts, nil))
}

func TestRewrite(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)

	// create backup
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, BackupOptions{}, env.gopts)
	snapshotIDs := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 1, "expected one snapshot, got %v", snapshotIDs)
	testRunCheck(t, env.gopts)

	// exclude some data
	testRunRewriteExclude(t, env.gopts, []string{"3"})
	snapshotIDs = testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 2, "expected two snapshots, got %v", snapshotIDs)
	testRunCheck(t, env.gopts)
}
