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

	rtest.OK(t, withTermStatus(gopts, func(ctx context.Context, term ui.Terminal) error {
		return runRewrite(context.TODO(), opts, gopts, nil, term)
	}))
}

func createBasicRewriteRepo(t testing.TB, env *testEnvironment) restic.ID {
	testSetupBackupData(t, env)

	// create backup
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, BackupOptions{}, env.gopts)
	snapshotIDs := testRunList(t, env.gopts, "snapshots")
	rtest.Assert(t, len(snapshotIDs) == 1, "expected one snapshot, got %v", snapshotIDs)
	testRunCheck(t, env.gopts)

	return snapshotIDs[0]
}

func getSnapshot(t testing.TB, snapshotID restic.ID, env *testEnvironment) *restic.Snapshot {
	t.Helper()

	var snapshots []*restic.Snapshot
	err := withTermStatus(env.gopts, func(ctx context.Context, term ui.Terminal) error {
		printer := newTerminalProgressPrinter(env.gopts.JSON, env.gopts.verbosity, term)
		ctx, repo, unlock, err := openWithReadLock(ctx, env.gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		snapshots, err = restic.TestLoadAllSnapshots(ctx, repo, nil)
		return err
	})
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
	testRunRewriteExclude(t, env.gopts, []string{"3"}, false, snapshotMetadataArgs{Hostname: "", Time: ""})
	snapshotIDs := testRunList(t, env.gopts, "snapshots")
	rtest.Assert(t, len(snapshotIDs) == 2, "expected two snapshots, got %v", snapshotIDs)
	testRunCheck(t, env.gopts)
}

func TestRewriteUnchanged(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	snapshotID := createBasicRewriteRepo(t, env)

	// use an exclude that will not exclude anything
	testRunRewriteExclude(t, env.gopts, []string{"3dflkhjgdflhkjetrlkhjgfdlhkj"}, false, snapshotMetadataArgs{Hostname: "", Time: ""})
	newSnapshotIDs := testRunList(t, env.gopts, "snapshots")
	rtest.Assert(t, len(newSnapshotIDs) == 1, "expected one snapshot, got %v", newSnapshotIDs)
	rtest.Assert(t, snapshotID == newSnapshotIDs[0], "snapshot id changed unexpectedly")
	testRunCheck(t, env.gopts)
}

func TestRewriteReplace(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	snapshotID := createBasicRewriteRepo(t, env)

	snapshot := getSnapshot(t, snapshotID, env)

	// exclude some data
	testRunRewriteExclude(t, env.gopts, []string{"3"}, true, snapshotMetadataArgs{Hostname: "", Time: ""})
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

	var snapshots []*restic.Snapshot
	err := withTermStatus(env.gopts, func(ctx context.Context, term ui.Terminal) error {
		printer := newTerminalProgressPrinter(env.gopts.JSON, env.gopts.verbosity, term)
		ctx, repo, unlock, err := openWithReadLock(ctx, env.gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		snapshots, err = restic.TestLoadAllSnapshots(ctx, repo, nil)
		return err
	})
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

func TestRewriteSnaphotSummary(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	createBasicRewriteRepo(t, env)

	rtest.OK(t, withTermStatus(env.gopts, func(ctx context.Context, term ui.Terminal) error {
		return runRewrite(context.TODO(), RewriteOptions{SnapshotSummary: true}, env.gopts, []string{}, term)
	}))
	// no new snapshot should be created as the snapshot already has a summary
	snapshots := testListSnapshots(t, env.gopts, 1)

	// replace snapshot by one without a summary
	var oldSummary *restic.SnapshotSummary
	err := withTermStatus(env.gopts, func(ctx context.Context, term ui.Terminal) error {
		printer := newTerminalProgressPrinter(env.gopts.JSON, env.gopts.verbosity, term)
		_, repo, unlock, err := openWithExclusiveLock(ctx, env.gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		sn, err := restic.LoadSnapshot(ctx, repo, snapshots[0])
		rtest.OK(t, err)
		oldSummary = sn.Summary
		sn.Summary = nil
		rtest.OK(t, repo.RemoveUnpacked(ctx, restic.WriteableSnapshotFile, snapshots[0]))
		snapshots[0], err = restic.SaveSnapshot(ctx, repo, sn)
		return err
	})
	rtest.OK(t, err)

	// rewrite snapshot and lookup ID of new snapshot
	rtest.OK(t, withTermStatus(env.gopts, func(ctx context.Context, term ui.Terminal) error {
		return runRewrite(context.TODO(), RewriteOptions{SnapshotSummary: true}, env.gopts, []string{}, term)
	}))
	newSnapshots := testListSnapshots(t, env.gopts, 2)
	newSnapshot := restic.NewIDSet(newSnapshots...).Sub(restic.NewIDSet(snapshots...)).List()[0]

	newSn := testLoadSnapshot(t, env.gopts, newSnapshot)
	rtest.Assert(t, newSn.Summary != nil, "snapshot should have summary attached")
	rtest.Equals(t, oldSummary.TotalBytesProcessed, newSn.Summary.TotalBytesProcessed, "unexpected TotalBytesProcessed value")
	rtest.Equals(t, oldSummary.TotalFilesProcessed, newSn.Summary.TotalFilesProcessed, "unexpected TotalFilesProcessed value")
}
