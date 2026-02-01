package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
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

func TestFindOldestNewest(t *testing.T) {
	if runtime.GOOS == "windows" {
		// windows does things differently
		return
	}
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testSetupBackupData(t, env)

	// setup test directory 0/0/9
	dir009 := filepath.Join(env.testdata, "0", "0", "9")
	dirEntries, err := os.ReadDir(dir009)
	rtest.OK(t, err)

	// select one random files and changes its Modtime
	timeStart := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local)
	timeEnded := time.Date(2025, 12, 31, 0, 0, 0, 0, time.Local)
	secondsIn2025 := int(timeEnded.Sub(timeStart).Seconds())
	numberOfFiles := len(dirEntries)

	pathName := filepath.Join(dir009, dirEntries[rand.Intn(numberOfFiles)].Name())
	modTimeFile := timeStart.Add(time.Second * time.Duration(rand.Intn(secondsIn2025)))
	rtest.OK(t, os.Chtimes(pathName, modTimeFile, modTimeFile))

	// backup
	testRunBackup(t, "", []string{dir009}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	// find
	findOpts := FindOptions{
		Oldest: "2025-01-01",
		Newest: "2025-12-31",
	}
	results := testRunFind(t, true, findOpts, env.gopts, pathName)

	matches := []testMatches{}
	rtest.OK(t, json.Unmarshal(results, &matches))
	rtest.Assert(t, len(matches) == 1, "expected one match, got %d", len(matches))
	rtest.Assert(t, matches[0].Hits == 1, "expected one hit, found hits=%d", matches[0].Hits)

	findOpts = FindOptions{
		Oldest: "2024-01-01",
		Newest: "2024-12-31",
	}
	results = testRunFind(t, true, findOpts, env.gopts, pathName)
	matches = []testMatches{}
	rtest.OK(t, json.Unmarshal(results, &matches))
	rtest.Assert(t, len(matches) == 0, "expected 0 matches for file, got %d", len(matches))
}

type findResult struct {
	ObjectType string    `json:"object_type"`
	ID         string    `json:"id"`
	Path       string    `json:"path"`
	ParentTree string    `json:"parent_tree,omitempty"`
	SnapshotID string    `json:"snapshot"`
	Time       time.Time `json:"time,omitempty"`
}

func TestFindBlobID(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)

	// setup test directory 0/0/9
	dir009 := filepath.Join(env.testdata, "0", "0", "9")
	dirEntries, err := os.ReadDir(dir009)
	rtest.OK(t, err)
	numberOfFiles := len(dirEntries)

	pathName := filepath.Join(dir009, dirEntries[rand.Intn(numberOfFiles)].Name())
	contents, err := os.ReadFile(pathName)
	rtest.OK(t, err)
	blobIDInBinary := sha256.Sum256(contents)
	blobID := hex.EncodeToString(blobIDInBinary[:])

	// backup
	testRunBackup(t, "", []string{dir009}, BackupOptions{}, env.gopts)
	sn := testListSnapshots(t, env.gopts, 1)
	// find
	out := testRunFind(t, true, FindOptions{BlobID: true}, env.gopts, blobID)

	findRes := []findResult{}
	rtest.OK(t, json.Unmarshal(out, &findRes))
	rtest.Assert(t, len(findRes) == 1, "expected one element, go %d", len(findRes))
	result := findRes[0]

	rtest.Assert(t,
		result.ObjectType == "blob" &&
			blobID == result.ID &&
			sn[0].String() == result.SnapshotID,
		"\nexpected ObjectType %s <=> %s\nexpected ID %s <=> %s\n expected snapshotID %s <=> %s",
		"blob", result.ObjectType,
		blobID, result.ID,
		sn[0].String(), result.SnapshotID,
	)
	if runtime.GOOS != "windows" {
		// windows pathnames are different
		rtest.Assert(t, pathName == result.Path, "expected pathname %q in result, got %q", pathName, result.Path)
	}
}

func TestFindPackID(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testSetupBackupData(t, env)

	dir009 := filepath.Join(env.testdata, "0", "0", "9")
	dirEntries, err := os.ReadDir(dir009)
	rtest.OK(t, err)
	numberOfFiles := len(dirEntries)

	// backup
	testRunBackup(t, "", []string{dir009}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	// extract packfile ID from repository index
	dataPackID := restic.ID{}
	treePackID := restic.ID{}
	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		// load Index
		rtest.OK(t, repo.LoadIndex(ctx, nil))
		// go through all index entries and collect data and tree packfile(s)
		rtest.OK(t, repo.ListBlobs(ctx, func(blob restic.PackedBlob) {
			switch blob.Type {
			case restic.DataBlob:
				dataPackID = blob.PackID
			case restic.TreeBlob:
				treePackID = blob.PackID
			}
		}))
		return nil
	})
	rtest.OK(t, err)

	rtest.Assert(t, !dataPackID.IsNull(), "expected to find data packfile in repo")
	packID := dataPackID.String()
	out := testRunFind(t, true, FindOptions{PackID: true}, env.gopts, packID)

	findRes := []findResult{}
	rtest.OK(t, json.Unmarshal(out, &findRes))
	rtest.Assert(t, len(findRes) == numberOfFiles, "expected %d entries for this packfile, got %d",
		numberOfFiles, len(findRes))

	// TODO: the tests for tree packfiles have to wait until PR#5664 has been resolved
	_ = treePackID
}
