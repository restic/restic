package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunRewriteExclude(t testing.TB, gopts GlobalOptions, excludes []string, forget bool, metadata snapshotMetadataArgs) {
	opts := RewriteOptions{
		excludePatternOptions: excludePatternOptions{
			Excludes: excludes,
		},
		Forget:   forget,
		Metadata: metadata,
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
	testRunRewriteExclude(t, env.gopts, []string{"3"}, false, snapshotMetadataArgs{Hostname: "", Time: ""})
	snapshotIDs := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 2, "expected two snapshots, got %v", snapshotIDs)
	testRunCheck(t, env.gopts)
}

func TestRewriteUnchanged(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	snapshotID := createBasicRewriteRepo(t, env)

	// use an exclude that will not exclude anything
	testRunRewriteExclude(t, env.gopts, []string{"3dflkhjgdflhkjetrlkhjgfdlhkj"}, false, snapshotMetadataArgs{Hostname: "", Time: ""})
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
	testRunRewriteExclude(t, env.gopts, []string{"3"}, true, snapshotMetadataArgs{Hostname: "", Time: ""})
	newSnapshotIDs := testListSnapshots(t, env.gopts, 1)
	rtest.Assert(t, snapshotID != newSnapshotIDs[0], "snapshot id should have changed")
	// check forbids unused blobs, thus remove them first
	testRunPrune(t, env.gopts, PruneOptions{MaxUnused: "0"})
	testRunCheck(t, env.gopts)
}

func testRewriteMetadata(t *testing.T, metadata snapshotMetadataArgs) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	createBasicRewriteRepo(t, env)
	testRunRewriteExclude(t, env.gopts, []string{}, true, metadata)

	repo, _ := OpenRepository(context.TODO(), env.gopts)
	snapshots, err := restic.TestLoadAllSnapshots(context.TODO(), repo, nil)
	rtest.OK(t, err)
	rtest.Assert(t, len(snapshots) == 1, "expected one snapshot, got %v", len(snapshots))
	newSnapshot := snapshots[0]

	if metadata.Time != "" {
		rtest.Assert(t, newSnapshot.Time.Format(TimeFormat) == metadata.Time, "New snapshot should have time %s", metadata.Time)
	}

	if metadata.Hostname != "" {
		rtest.Assert(t, newSnapshot.Hostname == metadata.Hostname, "New snapshot should have host %s", metadata.Hostname)
	}
}

func TestRewriteMetadata(t *testing.T) {
	newHost := "new host"
	newTime := "1999-01-01 11:11:11"

	for _, metadata := range []snapshotMetadataArgs{
		{Hostname: "", Time: newTime},
		{Hostname: newHost, Time: ""},
		{Hostname: newHost, Time: newTime},
	} {
		testRewriteMetadata(t, metadata)
	}
}
