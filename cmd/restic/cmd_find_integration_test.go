package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
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
	Date        time.Time `json:"date"`
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
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	sn := testListSnapshots(t, env.gopts, 1)[0]

	// second backup
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	snapshots := testListSnapshots(t, env.gopts, 2)
	// get id of new snapshot without depending on file order returned by filesystem
	sn2 := snapshots[0]
	if sn.Equal(sn2) {
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
	rtest.Equals(t, sn.String(), matchesReverse[0].SnapshotID, "snapshot[0] must match old snapshot")
	rtest.Equals(t, sn2.String(), matchesReverse[1].SnapshotID, "snapshot[1] must match new snapshot")
	rtest.Equals(t, matches[0].SnapshotID, matchesReverse[1].SnapshotID, "matches should be sorted 1")
	rtest.Equals(t, matches[1].SnapshotID, matchesReverse[0].SnapshotID, "matches should be sorted 2")
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
	Time       time.Time `json:"time"`
	Packfile   string    `json:"packfile,omitempty"`
}

func TestFindPackfile(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	backupPath := env.testdata + "/0/0/9"
	testRunBackup(t, "", []string{backupPath}, BackupOptions{}, env.gopts)
	sn := testListSnapshots(t, env.gopts, 1)[0]

	// do all the testing wrapped inside withTermStatus()
	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		// load master index
		rtest.OK(t, repo.LoadIndex(ctx, printer))

		packID := restic.ID{}
		done := false
		err = repo.ListBlobs(ctx, func(pb restic.PackBlob) {
			h := pb.Handle()
			if !done && h.Type == restic.TreeBlob {
				packID = pb.PackID()
				done = true
			}
		})
		rtest.OK(t, err)

		rtest.Assert(t, !packID.IsNull(), "expected a valid tree packfile ID")
		findOptions := FindOptions{PackID: true}
		results := testRunFind(t, true, findOptions, env.gopts, packID.String())

		// get the json records
		jsonResult := []JSONOutput{}
		rtest.OK(t, json.Unmarshal(results, &jsonResult))
		rtest.Assert(t, len(jsonResult) > 0, "expected at least one tree record in the packfile")

		// look at the last record
		record := jsonResult[len(jsonResult)-1]
		rtest.Equals(t, "tree", record.ObjectType, "expected a tree record type")
		rtest.Equals(t, sn.String(), record.SnapshotID, "expected snapshot ID being equal")
		backupPath = filepath.ToSlash(backupPath)[2:] // take the offending windows drive mapping away
		rtest.Assert(t, strings.Contains(record.Path, backupPath), "expected %q as part of %q", backupPath, record.Path)

		return nil
	})
	rtest.OK(t, err)
}

func TestFindPackID(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	testSetupBackupData(t, env)

	dir009 := filepath.Join(env.testdata, "0", "0", "9")
	dirEntries, err := os.ReadDir(dir009)
	rtest.OK(t, err)
	numberOfFiles := len(dirEntries)
	testRunBackup(t, "", []string{dir009}, BackupOptions{}, env.gopts)
	sn := testListSnapshots(t, env.gopts, 1)[0]

	dataPackID := restic.ID{}
	treePackID := restic.ID{}
	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		// load Index
		rtest.OK(t, repo.LoadIndex(ctx, restic.NoopTerminalCounterFactory))
		// go through all index entries and collect last data and last tree packfile(s)
		rtest.OK(t, repo.ListBlobs(ctx, func(blob restic.PackBlob) {
			switch blob.Handle().Type {
			case restic.DataBlob:
				dataPackID = blob.PackID()
			case restic.TreeBlob:
				treePackID = blob.PackID()
			}
		}))
		return nil
	})
	rtest.OK(t, err)

	// look for data packfile
	rtest.Assert(t, !dataPackID.IsNull(), "expected to find a data packfile in repo")
	packID := dataPackID.String()
	out := testRunFind(t, true, FindOptions{PackID: true}, env.gopts, packID)

	findRes := []JSONOutput{}
	rtest.OK(t, json.Unmarshal(out, &findRes))
	// the assumption is here that all data blobs fit in one data packfile
	rtest.Assert(t, len(findRes) == numberOfFiles, "expected %d entries for this packfile, got %d",
		numberOfFiles, len(findRes))

	// corrupt the data pack file; find must fall back to the index
	be := captureBackend(&env.gopts)
	err = withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, _, unlock, err := openWithExclusiveLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		h := backend.Handle{Type: backend.PackFile, Name: dataPackID.String()}
		rtest.OK(t, be().Remove(ctx, h))
		buf := make([]byte, 10)
		rtest.OK(t, be().Save(ctx, h, backend.NewByteReader(buf, be().Hasher())))
		return nil
	})
	rtest.OK(t, err)

	out = testRunFind(t, true, FindOptions{PackID: true}, env.gopts, dataPackID.String())
	findRes = []JSONOutput{}
	rtest.OK(t, json.Unmarshal(out, &findRes))
	rtest.Assert(t, len(findRes) == numberOfFiles, "expected %d entries for broken packfile, got %d",
		numberOfFiles, len(findRes))

	// look for tree packfile
	rtest.Assert(t, !treePackID.IsNull(), "expected to find at least one tree packfile in repo")
	packID = treePackID.String()
	out = testRunFind(t, true, FindOptions{PackID: true}, env.gopts, packID)

	findRes = []JSONOutput{}
	rtest.OK(t, json.Unmarshal(out, &findRes))
	record := findRes[len(findRes)-1]

	rtest.Equals(t, "tree", record.ObjectType)
	rtest.Equals(t, sn.String(), record.SnapshotID)
	rtest.Equals(t, filepath.ToSlash(dir009)[2:], filepath.ToSlash(record.Path)[2:])
}

func TestFindShowPackID(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	backupPath := filepath.Join(env.testdata, "0", "0", "9")
	testRunBackup(t, "", []string{backupPath}, BackupOptions{}, env.gopts)
	sn := testListSnapshots(t, env.gopts, 1)[0]

	err := withTermStatus(t, env.gopts, func(ctx context.Context, gopts global.Options) error {
		printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, gopts.Term)
		_, repo, unlock, err := openWithReadLock(ctx, gopts, false, printer)
		rtest.OK(t, err)
		defer unlock()

		rtest.OK(t, repo.LoadIndex(ctx, restic.NoopTerminalCounterFactory))

		doneT := false
		doneB := false
		dataBlob := restic.ID{}
		TPackID := restic.ID{}
		BPackID := restic.ID{}
		err = repo.ListBlobs(ctx, func(pb restic.PackBlob) {
			h := pb.Handle()
			switch h.Type {
			case restic.TreeBlob:
				if !doneT {
					TPackID = pb.PackID()
					doneT = true
				}
			case restic.DataBlob:
				if !doneB {
					BPackID = pb.PackID()
					dataBlob = pb.Handle().ID
					doneB = true
				}
			}
		})
		rtest.OK(t, err)

		rtest.Assert(t, !TPackID.IsNull(), "expected a tree packfile ID")
		rtest.Assert(t, !BPackID.IsNull(), "expected a data packfile ID")
		findOptions := FindOptions{PackID: true, ShowPackID: true}
		out := testRunFind(t, true, findOptions, env.gopts, TPackID.String())

		findRes := []JSONOutput{}
		rtest.OK(t, json.Unmarshal(out, &findRes))
		lastEntry := findRes[len(findRes)-1]
		rtest.Equals(t, TPackID.String(), lastEntry.Packfile, "packfile IDs should be identical")
		rtest.Equals(t, sn.String(), lastEntry.SnapshotID, "snapshot IDs should be identical")
		rtest.Equals(t, filepath.ToSlash(backupPath[2:]), filepath.ToSlash(lastEntry.Path[2:]), "pathnames should be identical")

		out = testRunFind(t, true, findOptions, env.gopts, BPackID.String())
		findRes = []JSONOutput{}
		rtest.OK(t, json.Unmarshal(out, &findRes))
		lastEntry = findRes[len(findRes)-1]
		rtest.Equals(t, BPackID.String(), lastEntry.Packfile, "packfile IDs should be identical")
		rtest.Equals(t, sn.String(), lastEntry.SnapshotID, "snapshot IDs should be identical")

		findOptions = FindOptions{BlobID: true, ShowPackID: true}
		out = testRunFind(t, true, findOptions, env.gopts, dataBlob.String())
		findRes = []JSONOutput{}
		rtest.OK(t, json.Unmarshal(out, &findRes))
		lastEntry = findRes[len(findRes)-1]
		rtest.Equals(t, BPackID.String(), lastEntry.Packfile, "packfile IDs should be identical")
		rtest.Equals(t, sn.String(), lastEntry.SnapshotID, "snapshot IDs should be identical")
		rtest.Equals(t, dataBlob.String(), lastEntry.ID, "datablob IDs should be identical")
		return nil
	})
	rtest.OK(t, err)
}

func TestFindOldestNewest(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	backupPath := filepath.Join(env.testdata, "/0")
	testRunBackup(t, "", []string{backupPath}, BackupOptions{}, env.gopts)
	sn := testListSnapshots(t, env.gopts, 1)[0]

	findOptions := FindOptions{Oldest: "2025-01-01", Newest: "2025-12-12"}
	out := testRunFind(t, true, findOptions, env.gopts, "*.txt")

	matches := []testMatches{}
	rtest.OK(t, json.Unmarshal(out, &matches))

	rtest.Assert(t, len(matches) == 1, "expected a single snapshot to match, but got %d", len(matches))
	first := matches[0]
	rtest.Assert(t, len(first.Matches) == 2, "expected two files to match")
	rtest.Assert(t, first.Hits == 2, "expected hits to show 2 matches")
	rtest.Equals(t, sn.String(), first.SnapshotID, "expected snapshots to be identical")
	rtest.Assert(t, strings.Contains(first.Matches[0].Path, ".txt"), "expected a text file, but got %q", first.Matches[0].Path)
}

// testRunFindMayFail is identical to testRunFind bar the fact it feeds back the error code
// and does not test it.
func testRunFindMayFail(t testing.TB, wantJSON bool, opts FindOptions, gopts global.Options, args []string) ([]byte, error) {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts global.Options) error {
		gopts.JSON = wantJSON

		return runFind(ctx, opts, gopts, args, gopts.Term)
	})
	return buf.Bytes(), err
}

func TestFindInvalidTreeID(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	backupPath := filepath.Join(env.testdata, "0")
	testRunBackup(t, "", []string{backupPath}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	findOptions := FindOptions{TreeID: true}
	_, err := testRunFindMayFail(t, false, findOptions, env.gopts, []string{"invalid-ID"})
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "unable to parse ID") &&
		strings.Contains(err.Error(), "invalid-ID"),
		"expected 'unable to parse ID', but got %v", err.Error())
}

func TestFindNoArgs(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	backupPath := filepath.Join(env.testdata, "0/for_cmd_ls")
	testRunBackup(t, "", []string{backupPath}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	findOptions := FindOptions{}
	_, err := testRunFindMayFail(t, false, findOptions, env.gopts, []string{})
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "wrong number of arguments"),
		"expected 'wrong number of arguments', but got %v", err)
}

func TestFindMultipleTypes(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	backupPath := filepath.Join(env.testdata, "0/for_cmd_ls")
	testRunBackup(t, "", []string{backupPath}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	findOptions := FindOptions{TreeID: true, BlobID: true}
	_, err := testRunFindMayFail(t, false, findOptions, env.gopts, []string{"abc"})
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "cannot have several ID types"),
		"expected 'cannot have several ID types', but got %v", err)
}

func TestFindWrongPackfile(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	backupPath := filepath.Join(env.testdata, "0/for_cmd_ls")
	testRunBackup(t, "", []string{backupPath}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	findOptions := FindOptions{PackID: true}
	_, err := testRunFindMayFail(t, false, findOptions, env.gopts, []string{"ccccccccaaaaaaaaffffffffffffffffeeeeeeeeeeeeeeee0000000011111111"})
	rtest.Assert(t, err != nil && strings.Contains(err.Error(),
		"unable to find pack(s)"), "expected 'unable to find pack(s)', but got %v", err)
}

func TestFindWrongTimeSpec(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	backupPath := filepath.Join(env.testdata, "0/for_cmd_ls")
	testRunBackup(t, "", []string{backupPath}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	findOptions := FindOptions{Oldest: "2025-01-0x"}
	_, err := testRunFindMayFail(t, false, findOptions, env.gopts, []string{"file"})
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "unable to parse time"),
		"expected 'unable to parse time', but got %v", err)
}

func TestFindCaseInsensitive(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	backupPath := filepath.Join(env.testdata, "/0")
	testRunBackup(t, "", []string{backupPath}, BackupOptions{}, env.gopts)
	sn := testListSnapshots(t, env.gopts, 1)[0]

	findOptions := FindOptions{Oldest: "2025-01-01", Newest: "2025-12-12", CaseInsensitive: true}
	out := testRunFind(t, true, findOptions, env.gopts, "*.TXT")

	matches := []testMatches{}
	rtest.OK(t, json.Unmarshal(out, &matches))

	rtest.Assert(t, len(matches) == 1, "expected a single snapshot to match, but got %d", len(matches))
	first := matches[0]
	rtest.Assert(t, len(first.Matches) == 2, "expected two filea to match")
	rtest.Assert(t, first.Hits == 2, "expected hits to show 2 matches")
	rtest.Equals(t, sn.String(), first.SnapshotID, "expected snapshot ID to be equal")
	rtest.Assert(t, strings.Contains(first.Matches[0].Path, ".txt"), "expected a text file, but got %q", first.Matches[0].Path)
}

func TestFindNotPreserveEmpty(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	baseDirectory := "0/for_cmd_ls"
	backupPath := filepath.Join(env.testdata, baseDirectory)
	rtest.OK(t, os.Mkdir(filepath.Join(env.testdata, baseDirectory, "empty-directory"), 0755))
	testRunBackup(t, "", []string{backupPath}, BackupOptions{}, env.gopts)

	findOptions := FindOptions{TreeID: true}
	_, err := testRunFindMayFail(t, true, findOptions, env.gopts, []string{"ac08ce34ba4f8123618661bef2425f7028ffb9ac740578a3ee88684d2523fee8"})
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "enable --preserve-empty-directory"), "expected error, got none")
}

func TestFindPreserveEmptyDirectory(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	baseDirectory := "0/for_cmd_ls"
	backupPath := filepath.Join(env.testdata, baseDirectory)
	rtest.OK(t, os.Mkdir(filepath.Join(env.testdata, baseDirectory, "empty-directory"), 0755))
	testRunBackup(t, "", []string{backupPath}, BackupOptions{}, env.gopts)

	findOptions := FindOptions{TreeID: true, PreserveEmptyDir: true}
	emptyDirectory := "ac08ce34ba4f8123618661bef2425f7028ffb9ac740578a3ee88684d2523fee8"
	out, err := testRunFindMayFail(t, false, findOptions, env.gopts, []string{emptyDirectory})
	rtest.OK(t, err)
	rtest.Assert(t, strings.Contains(string(out), emptyDirectory), "expected some output, got %q", out)
}
