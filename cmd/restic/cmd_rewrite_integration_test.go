package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui"
)

func testRunRewriteExclude(t testing.TB, gopts global.Options, excludes []string, forget bool, metadata snapshotMetadataArgs) {
	opts := RewriteOptions{
		ExcludePatternOptions: filter.ExcludePatternOptions{
			Excludes: excludes,
		},
		Forget:   forget,
		Metadata: metadata,
	}

	rtest.OK(t, withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runRewrite(context.TODO(), opts, gopts, nil, gopts.Term)
	}))
}

func testRunRewriteWithOpts(t testing.TB, opts RewriteOptions, gopts global.Options, args []string) error {
	rtest.OK(t, withTermStatus(t, gopts, func(ctx context.Context, gopts global.Options) error {
		return runRewrite(context.TODO(), opts, gopts, args, gopts.Term)
	}))
	return nil
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

func createBasicRewriteRepoWithEmptyDirectory(t testing.TB, env *testEnvironment) restic.ID {
	testSetupBackupData(t, env)

	// make an empty directory named "empty-directory"
	rtest.OK(t, os.Mkdir(filepath.Join(env.testdata, "/0/tests", "empty-directory"), 0755))

	// create backup
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, BackupOptions{}, env.gopts)
	snapshotIDs := testRunList(t, env.gopts, "snapshots")
	rtest.Assert(t, len(snapshotIDs) == 1, "expected one snapshot, got %v", snapshotIDs)

	return snapshotIDs[0]
}

func getSnapshot(t testing.TB, snapshotID restic.ID, env *testEnvironment) *data.Snapshot {
	t.Helper()

	var snapshots []*data.Snapshot
	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		ctx, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		snapshots, err = data.TestLoadAllSnapshots(ctx, repo, nil)
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

	var snapshots []*data.Snapshot
	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		ctx, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		snapshots, err = data.TestLoadAllSnapshots(ctx, repo, nil)
		return err
	})
	rtest.OK(t, err)
	rtest.Assert(t, len(snapshots) == 1, "expected one snapshot, got %v", len(snapshots))
	newSnapshot := snapshots[0]

	if metadata.Time != "" {
		rtest.Assert(t, newSnapshot.Time.Format(global.TimeFormat) == metadata.Time, "New snapshot should have time %s", metadata.Time)
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

	rtest.OK(t, withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runRewrite(context.TODO(), RewriteOptions{SnapshotSummary: true}, gopts, []string{}, gopts.Term)
	}))
	// no new snapshot should be created as the snapshot already has a summary
	snapshots := testListSnapshots(t, env.gopts, 1)

	// replace snapshot by one without a summary
	var oldSummary *data.SnapshotSummary
	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, repo, unlock, err := openWithExclusiveLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		sn, err := data.LoadSnapshot(ctx, repo, snapshots[0])
		rtest.OK(t, err)
		oldSummary = sn.Summary
		sn.Summary = nil
		rtest.OK(t, repo.RemoveUnpacked(ctx, restic.WriteableSnapshotFile, snapshots[0]))
		snapshots[0], err = data.SaveSnapshot(ctx, repo, sn)
		return err
	})
	rtest.OK(t, err)

	// rewrite snapshot and lookup ID of new snapshot
	rtest.OK(t, withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runRewrite(context.TODO(), RewriteOptions{SnapshotSummary: true}, gopts, []string{}, gopts.Term)
	}))
	newSnapshots := testListSnapshots(t, env.gopts, 2)
	newSnapshot := restic.NewIDSet(newSnapshots...).Sub(restic.NewIDSet(snapshots...)).List()[0]

	newSn := testLoadSnapshot(t, env.gopts, newSnapshot)
	rtest.Assert(t, newSn.Summary != nil, "snapshot should have summary attached")
	rtest.Equals(t, oldSummary.TotalBytesProcessed, newSn.Summary.TotalBytesProcessed, "unexpected TotalBytesProcessed value")
	rtest.Equals(t, oldSummary.TotalFilesProcessed, newSn.Summary.TotalFilesProcessed, "unexpected TotalFilesProcessed value")
}

func TestRewriteIncludeFiles(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	// opens repo, creates one backup of the whole lot of 'testdata'
	createBasicRewriteRepo(t, env)
	snapshots := testListSnapshots(t, env.gopts, 1)

	// include txt files
	err := testRunRewriteWithOpts(t,
		RewriteOptions{
			Forget:                true,
			IncludePatternOptions: filter.IncludePatternOptions{Includes: []string{"*.txt"}},
		},
		env.gopts,
		[]string{"latest"})
	rtest.OK(t, err)
	newSnapshots := testListSnapshots(t, env.gopts, 1)
	rtest.Assert(t, snapshots[0] != newSnapshots[0], "snapshot id should have changed")

	// read restic ls output and count .txt files
	count := 0
	out := testRunLsWithOpts(t, env.gopts, LsOptions{}, []string{"latest"})
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, ".txt") {
			count++
		}
	}
	rtest.Assert(t, count == 2, "expected two txt files, but got %d files", count)

	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, repo, unlock, err := openWithExclusiveLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		sn, err := data.LoadSnapshot(context.TODO(), repo, newSnapshots[0])
		rtest.OK(t, err)

		rtest.Assert(t, sn.Summary != nil, "snapshot should have a summary attached")
		rtest.Assert(t, sn.Summary.TotalFilesProcessed == 2,
			"there should be 2 files in the snapshot, but there are %d files", sn.Summary.TotalFilesProcessed)
		return nil
	})
	rtest.OK(t, err)
}

func TestRewriteExcludeFiles(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	createBasicRewriteRepo(t, env)
	snapshots := testListSnapshots(t, env.gopts, 1)

	// exclude txt files
	err := testRunRewriteWithOpts(t,
		RewriteOptions{
			Forget:                true,
			ExcludePatternOptions: filter.ExcludePatternOptions{Excludes: []string{"*.txt"}},
		},
		env.gopts,
		[]string{"latest"})
	rtest.OK(t, err)
	newSnapshots := testListSnapshots(t, env.gopts, 1)
	rtest.Assert(t, snapshots[0] != newSnapshots[0], "snapshot id should have changed")

	// read restic ls output and count .txt files
	count := 0
	out := testRunLsWithOpts(t, env.gopts, LsOptions{}, []string{"latest"})
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, ".txt") {
			count++
		}
	}
	rtest.Assert(t, count == 0, "expected 0 txt files, but got %d files", count)
}

func TestRewriteExcludeIncludeContradiction(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	createBasicRewriteRepo(t, env)
	testListSnapshots(t, env.gopts, 1)

	// test contradiction
	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runRewrite(ctx,
			RewriteOptions{
				ExcludePatternOptions: filter.ExcludePatternOptions{Excludes: []string{"nonsense"}},
				IncludePatternOptions: filter.IncludePatternOptions{Includes: []string{"not allowed"}},
			},
			gopts, []string{"quack"}, env.gopts.Term)
	})
	rtest.Assert(t, err != nil, `expected to fail command with message "exclude and include patterns are mutually exclusive"`)
}

func TestRewriteIncludeEmptyDirectory(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	snapIDEmpty := createBasicRewriteRepoWithEmptyDirectory(t, env)

	// restic rewrite <snapshots[0]> -i empty-directory --forget
	// exclude txt files
	err := testRunRewriteWithOpts(t,
		RewriteOptions{
			Forget:                true,
			IncludePatternOptions: filter.IncludePatternOptions{Includes: []string{"empty-directory"}},
		},
		env.gopts,
		[]string{"latest"})
	rtest.OK(t, err)
	newSnapshots := testListSnapshots(t, env.gopts, 1)
	rtest.Assert(t, snapIDEmpty != newSnapshots[0], "snapshot id should have changed")

	// read restic ls output and count lines with "empty-directory"
	count := 0
	out := testRunLsWithOpts(t, env.gopts, LsOptions{}, []string{"latest"})
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "empty-directory") {
			count++
		}
	}
	rtest.Assert(t, count == 1, "expected 1 empty directory, but got %d entries", count)
}

// TestRewriteIncludeNothing makes sure when nothing is included, the original snapshot stays untouched
func TestRewriteIncludeNothing(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	createBasicRewriteRepo(t, env)
	snapsBefore := testListSnapshots(t, env.gopts, 1)

	// restic rewrite latest -i nothing-whatsoever --forget
	err := testRunRewriteWithOpts(t,
		RewriteOptions{
			Forget:                true,
			ExcludePatternOptions: filter.ExcludePatternOptions{Excludes: []string{"nothing-whatsoever"}},
		},
		env.gopts,
		[]string{"latest"})
	rtest.OK(t, err)

	snapsAfter := testListSnapshots(t, env.gopts, 1)
	rtest.Assert(t, snapsBefore[0] == snapsAfter[0], "snapshots should be identical but are %s and %s",
		snapsBefore[0].Str(), snapsAfter[0].Str())
}

// TestRewriteExcludeNothing makes sure when nothing is excluded, the original snapshot stays untouched
// and no new (unchanged) snapshot would be created
func TestRewriteExcludeNothing(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	createBasicRewriteRepo(t, env)
	snapsBefore := testListSnapshots(t, env.gopts, 1)

	// restic rewrite latest -e 'nothing-whatsoever' --forget
	err := testRunRewriteWithOpts(t,
		RewriteOptions{
			Forget:                true,
			ExcludePatternOptions: filter.ExcludePatternOptions{Excludes: []string{"nothing-whatsoever"}},
		},
		env.gopts,
		[]string{"latest"})
	rtest.OK(t, err)
	snapsAfter := testListSnapshots(t, env.gopts, 1)
	rtest.Assert(t, snapsBefore[0] == snapsAfter[0], "snapshots should be identical but are %s and %s",
		snapsBefore[0].String(), snapsAfter[0].String())
}
