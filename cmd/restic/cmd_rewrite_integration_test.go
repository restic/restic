package main

import (
	"context"
	"path/filepath"
	"strings"
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

func testRunLsWithOptsOutput(t *testing.T, gopts GlobalOptions, newSnapshots []restic.ID,
	tester string, expected int) {
	out := testRunLsWithOpts(t, gopts, LsOptions{}, []string{newSnapshots[0].String()})
	fileList := strings.Split(string(out), "\n")
	count := 0
	for _, filename := range fileList {
		if strings.Contains(filename, tester) {
			count++
		}
	}
	rtest.Assert(t, count == expected, "there should be %d files/dirs in the snapshot, but there are %d files/dirs", expected, count)
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

	rtest.OK(t, runRewrite(context.TODO(), RewriteOptions{SnapshotSummary: true}, env.gopts, []string{}))
	// no new snapshot should be created as the snapshot already has a summary
	snapshots := testListSnapshots(t, env.gopts, 1)

	// replace snapshot by one without a summary
	_, repo, unlock, err := openWithExclusiveLock(context.TODO(), env.gopts, false)
	rtest.OK(t, err)
	sn, err := restic.LoadSnapshot(context.TODO(), repo, snapshots[0])
	rtest.OK(t, err)
	oldSummary := sn.Summary
	sn.Summary = nil
	rtest.OK(t, repo.RemoveUnpacked(context.TODO(), restic.WriteableSnapshotFile, snapshots[0]))
	snapshots[0], err = restic.SaveSnapshot(context.TODO(), repo, sn)
	rtest.OK(t, err)
	unlock()

	// rewrite snapshot and lookup ID of new snapshot
	rtest.OK(t, runRewrite(context.TODO(), RewriteOptions{SnapshotSummary: true}, env.gopts, []string{}))
	newSnapshots := testListSnapshots(t, env.gopts, 2)
	newSnapshot := restic.NewIDSet(newSnapshots...).Sub(restic.NewIDSet(snapshots...)).List()[0]

	sn, err = restic.LoadSnapshot(context.TODO(), repo, newSnapshot)
	rtest.OK(t, err)
	rtest.Assert(t, sn.Summary != nil, "snapshot should have summary attached")
	rtest.Equals(t, oldSummary.TotalBytesProcessed, sn.Summary.TotalBytesProcessed, "unexpected TotalBytesProcessed value")
}

func TestRewriteIncludeFiles(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	createBasicRewriteRepo(t, env)
	snapshots := testListSnapshots(t, env.gopts, 1)

	// restic rewrite <snapshots[0]> -i "*.txt" --forget
	rtest.OK(t, runRewrite(context.TODO(), RewriteOptions{
		Forget:                true,
		IncludePatternOptions: filter.IncludePatternOptions{Includes: []string{"*.txt"}}},
		env.gopts,
		[]string{snapshots[0].String()}))
	newSnapshots := testListSnapshots(t, env.gopts, 1)
	rtest.Assert(t, snapshots[0] != newSnapshots[0], "snapshot id should have changed")

	// read restic ls output and count .txt files
	testRunLsWithOptsOutput(t, env.gopts, newSnapshots, ".txt", 2)

	// get snapshot summary and find these 2 files
	_, repo, unlock, err := openWithExclusiveLock(context.TODO(), env.gopts, false)
	rtest.OK(t, err)
	defer unlock()

	sn, err := restic.LoadSnapshot(context.TODO(), repo, newSnapshots[0])
	rtest.OK(t, err)

	rtest.Assert(t, sn.Summary != nil, "snapshot should have a summary attached")
	rtest.Assert(t, sn.Summary.TotalFilesProcessed == 2,
		"there should be 2 files in the snapshot, but there are %d files", sn.Summary.TotalFilesProcessed)
}

func TestRewriteIncludeEmptyDirectory(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	createBasicRewriteRepo(t, env)
	snapshots := testListSnapshots(t, env.gopts, 1)

	// restic rewrite <snapshots[0]> -i empty-directory --forget
	rtest.OK(t, runRewrite(context.TODO(), RewriteOptions{
		Forget:                true,
		IncludePatternOptions: filter.IncludePatternOptions{Includes: []string{"empty-directory"}}},
		env.gopts,
		[]string{snapshots[0].Str()}))
	newSnapshots := testListSnapshots(t, env.gopts, 1)
	rtest.Assert(t, snapshots[0] != newSnapshots[0], "snapshot id should have changed")

	// read `restic ls` and count lines which contain the string "empty-directory"
	testRunLsWithOptsOutput(t, env.gopts, newSnapshots, "empty-directory", 1)

	// get snapshot summary and find this 1 directory
	_, repo, unlock, err := openWithExclusiveLock(context.TODO(), env.gopts, false)
	rtest.OK(t, err)
	defer unlock()

	sn, err := restic.LoadSnapshot(context.TODO(), repo, newSnapshots[0])
	rtest.OK(t, err)

	rtest.Assert(t, sn.Summary != nil, "snapshot should have a summary attached")
	rtest.Assert(t, sn.Summary.TotalFilesProcessed == 0,
		"there should be 0 files in the snapshot, but there are %d files", sn.Summary.TotalFilesProcessed)
	rtest.Assert(t, *sn.ID() == newSnapshots[0] && newSnapshots[0].String() != snapshots[0].String(),
		"snapshot should have changed, but is %s", snapshots[0].String())
}

func TestRewriteConflictingOptions(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	createBasicRewriteRepo(t, env)

	err := runRewrite(context.TODO(), RewriteOptions{}, env.gopts, []string{"latest"})
	rtest.Assert(t, err != nil, "Nothing to do: no includes/excludes provided and no new metadata provided")

	err = runRewrite(context.TODO(), RewriteOptions{
		IncludePatternOptions: filter.IncludePatternOptions{Includes: []string{"JohannSebastianBach"}},
		ExcludePatternOptions: filter.ExcludePatternOptions{Excludes: []string{"WolfgangAmadeusMozart"}},
	}, env.gopts, []string{"latest"})
	rtest.Assert(t, err != nil, "exclude and include patterns are mutually exclusive")
}

func TestRewriteIncludeNothing(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	createBasicRewriteRepo(t, env)
	snapsBefore := testListSnapshots(t, env.gopts, 1)

	// restic rewrite latest -i nothing-whatsoever --forget
	rtest.OK(t, runRewrite(context.TODO(), RewriteOptions{
		Forget:                true,
		IncludePatternOptions: filter.IncludePatternOptions{Includes: []string{"nothing-whatsoever"}}},
		env.gopts,
		[]string{"latest"}))
	snapsAfter := testListSnapshots(t, env.gopts, 1)
	rtest.Assert(t, snapsBefore[0] == snapsAfter[0], "snapshots should be identical but are %s and %s",
		snapsBefore[0].Str(), snapsAfter[0].Str())
}

func TestRewriteExcludeNothing(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	createBasicRewriteRepo(t, env)
	snapsBefore := testListSnapshots(t, env.gopts, 1)

	// restic rewrite latest -e 'nothing-whatsoever' --forget
	rtest.OK(t, runRewrite(context.TODO(), RewriteOptions{
		Forget:                true,
		ExcludePatternOptions: filter.ExcludePatternOptions{Excludes: []string{"nothing-whatsoever"}}},
		env.gopts,
		[]string{"latest"}))
	snapsAfter := testListSnapshots(t, env.gopts, 1)
	rtest.Assert(t, snapsBefore[0] == snapsAfter[0], "snapshots should be identical but are %s and %s",
		snapsBefore[0].Str(), snapsAfter[0].Str())
}

func TestRewriteExcludeAndCount(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	createBasicRewriteRepo(t, env)
	testListSnapshots(t, env.gopts, 1)

	// restic rewrite latest -e '0/0/7/' --forget
	rtest.OK(t, runRewrite(context.TODO(), RewriteOptions{
		Forget:                true,
		ExcludePatternOptions: filter.ExcludePatternOptions{Excludes: []string{"0/0/7/"}}},
		env.gopts,
		[]string{"latest"}))
	newSnapshots := testListSnapshots(t, env.gopts, 1)
	testRunLsWithOptsOutput(t, env.gopts, newSnapshots, "/7/", 0)
}
