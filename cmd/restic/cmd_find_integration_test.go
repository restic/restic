package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui"
)

func testRunFind(t testing.TB, wantJSON bool, opts FindOptions, gopts global.Options, pattern string) []byte {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.JSON = wantJSON

		return runFind(ctx, opts, gopts, []string{pattern}, gopts.Term)
	})
	rtest.OK(t, err)
	return buf.Bytes()
}

func TestFind(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := testSetupBackupData(t, env)
	opts := BackupOptions{}

	testRunBackup(t, "", []string{env.testdata}, opts, env.gopts)

	results := testRunFind(t, false, FindOptions{}, env.gopts, "unexistingfile")
	rtest.Assert(t, len(results) == 0, "unexisting file found in repo (%v)", datafile)

	results = testRunFind(t, false, FindOptions{}, env.gopts, "testfile")
	lines := strings.Split(string(results), "\n")
	rtest.Assert(t, len(lines) == 2, "expected one file found in repo (%v)", datafile)

	results = testRunFind(t, false, FindOptions{}, env.gopts, "testfile*")
	lines = strings.Split(string(results), "\n")
	rtest.Assert(t, len(lines) == 4, "expected three files found in repo (%v)", datafile)
}

type testMatch struct {
	Path        string    `json:"path,omitempty"`
	Permissions string    `json:"permissions,omitempty"`
	Size        uint64    `json:"size,omitempty"`
	Date        time.Time `json:"date,omitempty"`
	UID         uint32    `json:"uid,omitempty"`
	GID         uint32    `json:"gid,omitempty"`
}

type testMatches struct {
	Hits       int         `json:"hits,omitempty"`
	SnapshotID string      `json:"snapshot,omitempty"`
	Matches    []testMatch `json:"matches,omitempty"`
}

func TestFindJSON(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := testSetupBackupData(t, env)
	opts := BackupOptions{}

	testRunBackup(t, "", []string{env.testdata}, opts, env.gopts)
	testRunCheck(t, env.gopts)
	snapshot, _ := testRunSnapshots(t, env.gopts)

	results := testRunFind(t, true, FindOptions{}, env.gopts, "unexistingfile")
	matches := []testMatches{}
	rtest.OK(t, json.Unmarshal(results, &matches))
	rtest.Assert(t, len(matches) == 0, "expected no match in repo (%v)", datafile)

	results = testRunFind(t, true, FindOptions{}, env.gopts, "testfile")
	rtest.OK(t, json.Unmarshal(results, &matches))
	rtest.Assert(t, len(matches) == 1, "expected a single snapshot in repo (%v)", datafile)
	rtest.Assert(t, len(matches[0].Matches) == 1, "expected a single file to match (%v)", datafile)
	rtest.Assert(t, matches[0].Hits == 1, "expected hits to show 1 match (%v)", datafile)

	results = testRunFind(t, true, FindOptions{}, env.gopts, "testfile*")
	rtest.OK(t, json.Unmarshal(results, &matches))
	rtest.Assert(t, len(matches) == 1, "expected a single snapshot in repo (%v)", datafile)
	rtest.Assert(t, len(matches[0].Matches) == 3, "expected 3 files to match (%v)", datafile)
	rtest.Assert(t, matches[0].Hits == 3, "expected hits to show 3 matches (%v)", datafile)

	results = testRunFind(t, true, FindOptions{TreeID: true}, env.gopts, snapshot.Tree.String())
	rtest.OK(t, json.Unmarshal(results, &matches))
	rtest.Assert(t, len(matches) == 1, "expected a single snapshot in repo (%v)", matches)
	rtest.Assert(t, len(matches[0].Matches) == 3, "expected 3 files to match (%v)", matches[0].Matches)
	rtest.Assert(t, matches[0].Hits == 3, "expected hits to show 3 matches (%v)", datafile)
}

func TestFindSorting(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}

	// first backup
	testRunBackup(t, "", []string{env.testdata}, opts, env.gopts)
	sn1 := testListSnapshots(t, env.gopts, 1)[0]

	// second backup
	testRunBackup(t, "", []string{env.testdata}, opts, env.gopts)
	snapshots := testListSnapshots(t, env.gopts, 2)
	// get id of new snapshot without depending on file order returned by filesystem
	sn2 := snapshots[0]
	if sn1.Equal(sn2) {
		sn2 = snapshots[1]
	}

	// first restic find - with default FindOptions{}
	results := testRunFind(t, true, FindOptions{}, env.gopts, "testfile")
	lines := strings.Split(string(results), "\n")
	rtest.Assert(t, len(lines) == 2, "expected two lines of output, found %d", len(lines))
	matches := []testMatches{}
	rtest.OK(t, json.Unmarshal(results, &matches))

	// run second restic find with --reverse, sort oldest to newest
	resultsReverse := testRunFind(t, true, FindOptions{Reverse: true}, env.gopts, "testfile")
	lines = strings.Split(string(resultsReverse), "\n")
	rtest.Assert(t, len(lines) == 2, "expected two lines of output, found %d", len(lines))
	matchesReverse := []testMatches{}
	rtest.OK(t, json.Unmarshal(resultsReverse, &matchesReverse))

	// compare result sets
	rtest.Assert(t, sn1.String() == matchesReverse[0].SnapshotID, "snapshot[0] must match old snapshot")
	rtest.Assert(t, sn2.String() == matchesReverse[1].SnapshotID, "snapshot[1] must match new snapshot")
	rtest.Assert(t, matches[0].SnapshotID == matchesReverse[1].SnapshotID, "matches should be sorted 1")
	rtest.Assert(t, matches[1].SnapshotID == matchesReverse[0].SnapshotID, "matches should be sorted 2")
}

func TestFindInvalidTimeRange(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	err := runFind(context.TODO(), FindOptions{Oldest: "2026-01-01", Newest: "2020-01-01"}, env.gopts, []string{"quack"}, env.gopts.Term)
	rtest.Assert(t, err != nil && err.Error() == "Fatal: --oldest must specify a time before --newest",
		"unexpected error message: %v", err)
}

// JsonOutput is the struct `restic find --json` produces
type JSONOutput struct {
	ObjectType string    `json:"object_type"`
	ID         string    `json:"id"`
	Path       string    `json:"path"`
	ParentTree string    `json:"parent_tree,omitempty"`
	SnapshotID string    `json:"snapshot"`
	Time       time.Time `json:"time,omitempty"`
}

func TestFindPackfile(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)

	// backup
	backupPath := env.testdata + "/0/0/9"
	testRunBackup(t, "", []string{backupPath}, BackupOptions{}, env.gopts)
	sn1 := testListSnapshots(t, env.gopts, 1)[0]

	// do all the testing wrapped inside withTermStatus()
	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		// load master index
		rtest.OK(t, repo.LoadIndex(ctx, printer))

		packID := restic.ID{}
		done := false
		err = repo.ListBlobs(ctx, func(pb restic.PackedBlob) {
			if !done && pb.Type == restic.TreeBlob {
				packID = pb.PackID
				done = true
			}
		})

		rtest.OK(t, err)
		rtest.Assert(t, !packID.IsNull(), "expected a tree packfile ID")
		findOptions := FindOptions{PackID: true}
		results := testRunFind(t, true, findOptions, env.gopts, packID.String())

		// get the json records
		jsonResult := []JSONOutput{}
		rtest.OK(t, json.Unmarshal(results, &jsonResult))
		rtest.Assert(t, len(jsonResult) > 0, "expected at least one tree record in the packfile")

		// look at the last record
		lastIndex := len(jsonResult) - 1
		record := jsonResult[lastIndex]
		rtest.Assert(t, record.ObjectType == "tree" && record.SnapshotID == sn1.String(),
			"expected a tree record with known snapshot id, but got type=%s and snapID=%s instead of %s",
			record.ObjectType, record.SnapshotID, sn1.String())
		backupPath = filepath.ToSlash(backupPath)[2:] // take the offending drive mapping away
		rtest.Assert(t, strings.Contains(record.Path, backupPath), "expected %q as part of %q", backupPath, record.Path)
		// Windows response:
		//expected "C:/Users/RUNNER~1/AppData/Local/Temp/restic-test-3529440698/testdata/0/0/9" as part of
		//         "/C/Users/RUNNER~1/AppData/Local/Temp/restic-test-3529440698/testdata/0/0/9"

		return nil
	})
	rtest.OK(t, err)
}
