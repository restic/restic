package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui"
)

func testRunRewriteExclude(t testing.TB, gopts GlobalOptions, excludes []string, forget bool, metadata snapshotMetadataArgs) {
	opts := RewriteOptions{
		ExcludePatternOptions: filter.ExcludePatternOptions{
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

func getSnapshot(t testing.TB, snapshotID restic.ID, env *testEnvironment) *restic.Snapshot {
	t.Helper()

	ctx, repo, unlock, err := openWithReadLock(context.TODO(), env.gopts, false)
	rtest.OK(t, err)
	defer unlock()

	snapshots, err := restic.TestLoadAllSnapshots(ctx, repo, nil)
	rtest.OK(t, err)

	for _, s := range snapshots {
		if *s.ID() == snapshotID {
			return s
		}
	}
	return nil
}

func TestRewrite(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	createBasicRewriteRepo(t, env)

	// exclude some data
	testRunRewriteExclude(t, env.gopts, []string{"3"}, false, snapshotMetadataArgs{})
	snapshotIDs := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 2, "expected two snapshots, got %v", snapshotIDs)
	testRunCheck(t, env.gopts)
}

func TestRewriteUnchanged(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	snapshotID := createBasicRewriteRepo(t, env)

	testRunRewriteExclude(t, env.gopts, []string{"3dflkhjgdflhkjetrlkhjgfdlhkj"}, false, snapshotMetadataArgs{})
	newSnapshotIDs := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(newSnapshotIDs) == 2, "expected two snapshots, got %v", newSnapshotIDs)
	rtest.Assert(t, snapshotID == newSnapshotIDs[0], "snapshot id changed unexpectedly")
	testRunCheck(t, env.gopts)
}

func TestRewriteReplace(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	snapshotID := createBasicRewriteRepo(t, env)

	snapshot := getSnapshot(t, snapshotID, env)

	// exclude some data
	testRunRewriteExclude(t, env.gopts, []string{"3"}, true, snapshotMetadataArgs{})
	bytesExcluded, err := ui.ParseBytes("16K")
	rtest.OK(t, err)

	newSnapshotIDs := testListSnapshots(t, env.gopts, 1)
	rtest.Assert(t, snapshotID != newSnapshotIDs[0], "snapshot id should have changed")

	newSnapshot := getSnapshot(t, newSnapshotIDs[0], env)

	rtest.Equals(t, snapshot.Summary.TotalFilesProcessed-1, newSnapshot.Summary.TotalFilesProcessed, "snapshot file count should have changed")
	rtest.Equals(t, snapshot.Summary.TotalBytesProcessed-uint64(bytesExcluded), newSnapshot.Summary.TotalBytesProcessed, "snapshot size should have changed")

	// check forbids unused blobs, thus remove them first
	testRunPrune(t, env.gopts, PruneOptions{MaxUnused: "0"})
	testRunCheck(t, env.gopts)
}

func testRewriteMetadata(t *testing.T, metadata snapshotMetadataArgs) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	createBasicRewriteRepo(t, env)
	testRunRewriteExclude(t, env.gopts, []string{}, true, metadata)

	ctx, repo, unlock, err := openWithReadLock(context.TODO(), env.gopts, false)
	rtest.OK(t, err)
	defer unlock()

	snapshots, err := restic.TestLoadAllSnapshots(ctx, repo, nil)
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
	newHost := "new-host" // hostname cannot contain spaces according to Linux !
	newTime := "1999-01-01 11:11:11"

	for _, metadata := range []snapshotMetadataArgs{
		//{Hostname: "", Time: newTime}, // hostname must be valid !
		//{Hostname: newHost, Time: ""}, // empty time string now not allowed any more !
		{Hostname: newHost, Time: newTime},
	} {
		testRewriteMetadata(t, metadata)
	}
}

func TestRewriteSnaphotSummary(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	snap := createBasicRewriteRepo(t, env)

	// rewrite --snapshot-summary <snap>
	rtest.OK(t, runRewrite(context.TODO(), RewriteOptions{SnapshotSummary: true}, env.gopts, []string{snap.String()}))
	// since we have attached SnapshotSummary to <snap>, the call to runRewrite is a no op
	testListSnapshots(t, env.gopts, 1)

	// get repo so we can access snapshot strcuture - this is a hackish.
	// Is there a better way of doing it?
	t.Helper()
	_, repo, unlock, err := openWithReadLock(context.TODO(), env.gopts, false)
	rtest.OK(t, err)
	defer unlock()

	sn, _, err := restic.FindSnapshot(context.TODO(), repo, repo, snap.String())
	rtest.OK(t, err)
	rtest.Assert(t, sn.Summary != nil, "snapshot should have summary attached")
	rtest.Assert(t, sn.Summary.TotalBytesProcessed > 0, "snapshot TotalBytesProcessed > 0")
}
