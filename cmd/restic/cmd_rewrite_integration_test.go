package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
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

func testRunRewriteWithOutput(t testing.TB, gopts global.Options, opts RewriteOptions, args []string, wantJSON bool) []byte {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.JSON = wantJSON
		return runRewrite(context.TODO(), opts, gopts, args, gopts.Term)
	})
	rtest.OK(t, err)
	return buf.Bytes()
}

// testLsOutputContainsCount runs restic ls with the given options and asserts that
// exactly expectedCount lines of the output contain substring.
func testLsOutputContainsCount(t testing.TB, gopts global.Options, lsOpts LsOptions, lsArgs []string, substring string, expectedCount int) {
	t.Helper()
	out := testRunLsWithOpts(t, gopts, lsOpts, lsArgs)
	count := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, substring) {
			count++
		}
	}
	rtest.Assert(t, count == expectedCount, "expected %d lines containing %q, but got %d", expectedCount, substring, count)
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
		printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
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
		printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
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
		printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
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

func TestRewriteInclude(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		opts                 RewriteOptions
		lsSubstring          string
		lsExpectedCount      int
		summaryFilesExpected uint
	}{
		{"relative", RewriteOptions{
			Forget:                true,
			IncludePatternOptions: filter.IncludePatternOptions{Includes: []string{"*.txt"}},
		}, ".txt", 2, 2},
		{"absolute", RewriteOptions{
			Forget: true,
			// test that childMatches are working by only matching a subdirectory
			IncludePatternOptions: filter.IncludePatternOptions{Includes: []string{"/testdata/0/for_cmd_ls"}},
		}, "/testdata/0", 5, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, cleanup := withTestEnvironment(t)
			defer cleanup()
			createBasicRewriteRepo(t, env)
			snapshots := testListSnapshots(t, env.gopts, 1)

			rtest.OK(t, testRunRewriteWithOpts(t, tc.opts, env.gopts, []string{"latest"}))

			newSnapshots := testListSnapshots(t, env.gopts, 1)
			rtest.Assert(t, snapshots[0] != newSnapshots[0], "snapshot id should have changed")

			testLsOutputContainsCount(t, env.gopts, LsOptions{}, []string{"latest"}, tc.lsSubstring, tc.lsExpectedCount)
			sn := testLoadSnapshot(t, env.gopts, newSnapshots[0])
			rtest.Assert(t, sn.Summary != nil, "snapshot should have a summary attached")
			rtest.Assert(t, sn.Summary.TotalFilesProcessed == tc.summaryFilesExpected,
				"there should be %d files in the snapshot, but there are %d files", tc.summaryFilesExpected, sn.Summary.TotalFilesProcessed)
		})
	}
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

	testLsOutputContainsCount(t, env.gopts, LsOptions{}, []string{"latest"}, ".txt", 0)
}

func TestRewriteExcludeIncludeContradiction(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testRunInit(t, env.gopts)

	// test contradiction
	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		return runRewrite(ctx,
			RewriteOptions{
				ExcludePatternOptions: filter.ExcludePatternOptions{Excludes: []string{"nonsense"}},
				IncludePatternOptions: filter.IncludePatternOptions{Includes: []string{"not allowed"}},
			},
			gopts, []string{"quack"}, env.gopts.Term)
	})
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "exclude and include patterns are mutually exclusive"), `expected to fail command with message "exclude and include patterns are mutually exclusive"`)
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

	testLsOutputContainsCount(t, env.gopts, LsOptions{}, []string{"latest"}, "empty-directory", 1)
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
			IncludePatternOptions: filter.IncludePatternOptions{Includes: []string{"nothing-whatsoever"}},
		},
		env.gopts,
		[]string{"latest"})
	rtest.OK(t, err)

	snapsAfter := testListSnapshots(t, env.gopts, 1)
	rtest.Assert(t, snapsBefore[0] == snapsAfter[0], "snapshots should be identical but are %s and %s",
		snapsBefore[0].Str(), snapsAfter[0].Str())
}

func TestRewriteJSONInclude(t *testing.T) {
	type MessageTester struct {
		MessageType string `json:"message_type"`
	}

	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	createBasicRewriteRepo(t, env)
	originalSnapshot := testListSnapshots(t, env.gopts, 1)

	// rewrite include *.py and *.txt files = 3 files, drop original snapshot = make counting easier
	// rewrite with verbosity
	includeOptions := RewriteOptions{
		Forget:                true,
		IncludePatternOptions: filter.IncludePatternOptions{Includes: []string{"*.py", "*.txt"}},
	}
	env.gopts.Verbosity = 2
	buf := testRunRewriteWithOutput(t, env.gopts, includeOptions, []string{"latest"}, true)
	modifiedSnapshot := testListSnapshots(t, env.gopts, 1)

	countInclude := 0
	for _, line := range bytes.Split(buf, []byte("\n")) {
		if len(line) == 0 {
			break
		}

		tester := &MessageTester{}
		rtest.OK(t, json.Unmarshal(line, &tester))
		switch tester.MessageType {
		case "included_item":
			countInclude++
		case "rewrite":
			matches := &RewriteJSON{}
			rtest.OK(t, json.Unmarshal(line, &matches))

			rtest.Equals(t, matches.SnapshotID, originalSnapshot[0])
			rtest.Equals(t, matches.NewSnapshotID, modifiedSnapshot[0])
			rtest.Equals(t, matches.TotalFilesProcessed, 3)
			rtest.Equals(t, matches.TotalBytesProcessed, 100+18+113)
		}
	}
	rtest.Equals(t, countInclude, 3)
}

func TestRewriteJSONExclude(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	createBasicRewriteRepo(t, env)
	originalSnapshot := testListSnapshots(t, env.gopts, 1)

	// rewrite exclude, exclude symlink because windows does things differently with symlinks
	excludeOptions := RewriteOptions{
		Forget:                true,
		ExcludePatternOptions: filter.ExcludePatternOptions{Excludes: []string{"0/0/9/*", "testfile-symlink"}},
	}
	buf := testRunRewriteWithOutput(t, env.gopts, excludeOptions, []string{"latest"}, true)
	modifiedSnapshot := testListSnapshots(t, env.gopts, 1)

	matches := &RewriteJSON{}
	rtest.OK(t, json.Unmarshal(buf, &matches))

	rtest.Equals(t, "rewrite", matches.MessageType)
	rtest.Equals(t, originalSnapshot[0], matches.SnapshotID)
	rtest.Equals(t, modifiedSnapshot[0], matches.NewSnapshotID)

	// NOTE: windows counts symbolic links differently
	if runtime.GOOS != "windows" {
		rtest.Equals(t, 6, matches.TotalFilesProcessed)
		rtest.Equals(t, 100+18+113+0+21+21, matches.TotalBytesProcessed)

		lsOpts := LsOptions{ListLong: true}
		testLsOutputContainsCount(t, env.gopts, lsOpts, []string{"latest"}, "-rw-r--r--", 6)
	}
}
