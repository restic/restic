package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunRewriteExclude(t testing.TB, gopts GlobalOptions, excludes []string, forget bool) {
	opts := RewriteOptions{
		excludePatternOptions: excludePatternOptions{
			Excludes: excludes,
		},
		Forget: forget,
	}

	rtest.OK(t, runRewrite(context.TODO(), opts, gopts, nil))
}

func createBasicRewriteRepo(t testing.TB, env *testEnvironment) restic.ID {
	testSetupBackupData(t, env)

	// create backup
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, BackupOptions{}, env.gopts)
	snapshotIDs := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 1, "expected one snapshot, got %v", snapshotIDs)
	testRunCheck(t, env.gopts)

	return snapshotIDs[0]
}

func TestRewrite(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	createBasicRewriteRepo(t, env)

	// exclude some data
	testRunRewriteExclude(t, env.gopts, []string{"3"}, false)
	snapshotIDs := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 2, "expected two snapshots, got %v", snapshotIDs)
	testRunCheck(t, env.gopts)
}

func TestRewriteUnchanged(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	snapshotID := createBasicRewriteRepo(t, env)

	// use an exclude that will not exclude anything
	testRunRewriteExclude(t, env.gopts, []string{"3dflkhjgdflhkjetrlkhjgfdlhkj"}, false)
	newSnapshotIDs := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(newSnapshotIDs) == 1, "expected one snapshot, got %v", newSnapshotIDs)
	rtest.Assert(t, snapshotID == newSnapshotIDs[0], "snapshot id changed unexpectedly")
	testRunCheck(t, env.gopts)
}

func TestRewriteReplace(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	snapshotID := createBasicRewriteRepo(t, env)

	// exclude some data
	testRunRewriteExclude(t, env.gopts, []string{"3"}, true)
	newSnapshotIDs := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(newSnapshotIDs) == 1, "expected one snapshot, got %v", newSnapshotIDs)
	rtest.Assert(t, snapshotID != newSnapshotIDs[0], "snapshot id should have changed")
	// check forbids unused blobs, thus remove them first
	testRunPrune(t, env.gopts, PruneOptions{MaxUnused: "0"})
	testRunCheck(t, env.gopts)
}
