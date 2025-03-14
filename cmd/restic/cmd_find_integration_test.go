package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/restic"
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
	ModTime     time.Time `json:"mtime,omitempty"`
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

func TestFindWrongOptions(t *testing.T) {

	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "7")}, opts, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	err := runFind(context.TODO(), FindOptions{Oldest: "2025-01-01", Newest: "2020-01-01"}, env.gopts, []string{"quack"})
	rtest.Assert(t, err != nil && err.Error() == "Fatal: option conflict: `--oldest` >= `--newest`",
		"Fatal: option conflict: `--oldest` >= `--newest`")

	err = runFind(context.TODO(), FindOptions{BlobID: true, TreeID: true}, env.gopts, []string{"quackquack"})
	rtest.Assert(t, err != nil && err.Error() == "Fatal: cannot have several ID types", "Fatal: cannot have several ID types")

	err = runFind(context.TODO(), FindOptions{BlobID: true, Newest: "2024"}, env.gopts, []string{"quackquackquack"})
	rtest.Assert(t, err != nil && err.Error() == "Fatal: You cannot mix modification time matching with ID matching",
		"Fatal: You cannot mix modification time matching with ID matching")
}

func TestFindMtimeCheck(t *testing.T) {

	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "7")}, opts, env.gopts)
	snList := testListSnapshots(t, env.gopts, 1)

	optsF := FindOptions{
		Oldest: "2020-01-01 00:00:00",
		Newest: "2020-12-31 23:59:59",
	}
	results := testRunFind(t, true, optsF, env.gopts, filepath.Join(env.testdata, "0", "0", "7"))

	matches := []testMatches{}
	rtest.OK(t, json.Unmarshal(results, &matches))
	rtest.Assert(t, len(matches) == 1, "expected a single snapshot in repo (%v)", datafile)
	rtest.Assert(t, matches[0].Hits == 3, "expected the files from the year 2020")
	rtest.Assert(t, matches[0].SnapshotID == snList[0].String(), "snapID should match")
	for _, hit := range matches[0].Matches {
		rtest.Assert(t, hit.ModTime.Year() == 2020, "should be a file from 2020")
	}
}

type FindBlobResult struct {
	// Add these attributes
	ObjectType string    `json:"object_type"`
	ID         string    `json:"id"`
	Path       string    `json:"path"`
	ParentTree string    `json:"parent_tree,omitempty"`
	SnapshotID string    `json:"snapshot"`
	Time       time.Time `json:"time,omitempty"`
}

func TestFindIDMatching(t *testing.T) {

	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "7")}, opts, env.gopts)
	snList := testListSnapshots(t, env.gopts, 1)

	_, repo, unlock, err := openWithReadLock(context.TODO(), env.gopts, false)
	rtest.OK(t, err)
	defer unlock()

	packToTest := ""
	rtest.OK(t, repo.LoadIndex(context.TODO(), nil))
	rtest.OK(t, repo.ListBlobs(context.TODO(), func(blob restic.PackedBlob) {
		if blob.Type == restic.TreeBlob {
			packToTest = blob.PackID.String()
			return
		}
	}))

	// pack testing for tree packfile
	optsF := FindOptions{
		PackID: true,
	}
	results := testRunFind(t, true, optsF, env.gopts, packToTest)
	tester := []printBuffer{}
	rtest.OK(t, json.Unmarshal(results, &tester))
	rtest.Assert(t, len(tester) == 7, "expected 7 JSON lines but got %d", len(tester))
	rtest.Assert(t, tester[0].SnapshotID == snList[0].String(), "expected snapID to be equal, but is %s", tester[0].SnapshotID[:8])
	rtest.Assert(t, tester[0].PackID.String() == packToTest, "expected packID to be equal, but is %s", tester[0].PackID.String())

	// pack testing for data packfile
	// get first data pack
	rtest.OK(t, repo.ListBlobs(context.TODO(), func(blob restic.PackedBlob) {
		if blob.Type == restic.DataBlob {
			packToTest = blob.PackID.String()
			return
		}
	}))

	optsF = FindOptions{
		PackID: true,
	}
	results = testRunFind(t, true, optsF, env.gopts, packToTest)
	tester = []printBuffer{}
	rtest.OK(t, json.Unmarshal(results, &tester))
	rtest.Assert(t, len(tester) == 51, "expected 51 JSON blobs but got %d", len(tester))
	rtest.Assert(t, tester[0].SnapshotID == snList[0].String(), "expected snapID to be equal, but is %s", tester[0].SnapshotID[:8])
	rtest.Assert(t, tester[0].PackID.String() == packToTest, "expected packID to be equal, but is %s", tester[0].PackID.Str())

	// tree ID matching
	treeToTest := ""
	// get first tree blob
	rtest.OK(t, repo.ListBlobs(context.TODO(), func(blob restic.PackedBlob) {
		if blob.Type == restic.TreeBlob {
			treeToTest = blob.ID.String()
			return
		}
	}))

	optsF = FindOptions{
		TreeID: true,
	}
	results = testRunFind(t, true, optsF, env.gopts, treeToTest)
	tester = []printBuffer{}
	rtest.OK(t, json.Unmarshal(results, &tester))
	rtest.Assert(t, len(tester) == 1, "expected one JSON line but got %d", len(tester))

	rtest.Assert(t, tester[0].SnapshotID == snList[0].String(), "expected snapID to be equal, but is %s", tester[0].SnapshotID[:8])
	rtest.Assert(t, tester[0].ID.String() == treeToTest, "expected id to be equal, but is %s", tester[0].ID.Str())

	// blob ID matching
	optsF = FindOptions{
		BlobID: true,
	}
	// data blob for testdata/0/0/7/50
	blobToTest := "18cac6eb4b52212610192324c3bd80f1cb4cd7de67e5ff3f4f18ab7cf754a4fd"
	results = testRunFind(t, true, optsF, env.gopts, blobToTest)
	tester = []printBuffer{}
	rtest.OK(t, json.Unmarshal(results, &tester))
	rtest.Assert(t, len(tester) == 1, "expected one JSON line")

	rtest.Assert(t, tester[0].SnapshotID == snList[0].String(), "expected snapID to be equal, but is %s", tester[0].SnapshotID[:8])
	rtest.Assert(t, tester[0].ID.String() == blobToTest, "expected id to be equal, but is %s", tester[0].ID.Str())
}
