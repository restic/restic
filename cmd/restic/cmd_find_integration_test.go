package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	rtest "github.com/restic/restic/internal/test"
)

func testRunFind(t testing.TB, wantJSON bool, opts FindOptions, gopts GlobalOptions, pattern string) []byte {
	buf, err := withCaptureStdout(func() error {
		gopts.JSON = wantJSON

		return runFind(context.TODO(), opts, gopts, []string{pattern})
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

	datafile := testSetupBackupData(t, env)
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
	rtest.Assert(t, len(lines) == 2, "expected two files found in repo (%v), found %d", datafile, len(lines))
	matches := []testMatches{}
	rtest.OK(t, json.Unmarshal(results, &matches))

	// run second restic find with --reverse, sort oldest to newest
	resultsReverse := testRunFind(t, true, FindOptions{Reverse: true}, env.gopts, "testfile")
	lines = strings.Split(string(resultsReverse), "\n")
	rtest.Assert(t, len(lines) == 2, "expected two files found in repo (%v), found %d", datafile, len(lines))
	matchesReverse := []testMatches{}
	rtest.OK(t, json.Unmarshal(resultsReverse, &matchesReverse))

	// compare result sets
	rtest.Assert(t, sn1.String() == matchesReverse[0].SnapshotID, "snapshot[0] must match old snapshot")
	rtest.Assert(t, sn2.String() == matchesReverse[1].SnapshotID, "snapshot[1] must match new snapshot")
	rtest.Assert(t, matches[0].SnapshotID == matchesReverse[1].SnapshotID, "matches should be sorted 1")
	rtest.Assert(t, matches[1].SnapshotID == matchesReverse[0].SnapshotID, "matches should be sorted 2")
}
