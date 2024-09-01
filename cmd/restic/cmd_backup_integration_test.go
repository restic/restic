package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/termstatus"
)

func testRunBackupAssumeFailure(t testing.TB, dir string, target []string, opts BackupOptions, gopts GlobalOptions) error {
	return withTermStatus(gopts, func(ctx context.Context, term *termstatus.Terminal) error {
		t.Logf("backing up %v in %v", target, dir)
		if dir != "" {
			cleanup := rtest.Chdir(t, dir)
			defer cleanup()
		}

		opts.GroupBy = restic.SnapshotGroupByOptions{Host: true, Path: true}
		return runBackup(ctx, opts, gopts, term, target)
	})
}

func testRunBackup(t testing.TB, dir string, target []string, opts BackupOptions, gopts GlobalOptions) {
	err := testRunBackupAssumeFailure(t, dir, target, opts, gopts)
	rtest.Assert(t, err == nil, "Error while backing up")
}

func TestBackup(t *testing.T) {
	testBackup(t, false)
}

func TestBackupWithFilesystemSnapshots(t *testing.T) {
	if runtime.GOOS == "windows" && fs.HasSufficientPrivilegesForVSS() == nil {
		testBackup(t, true)
	}
}

func testBackup(t *testing.T, useFsSnapshot bool) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{UseFsSnapshot: useFsSnapshot}

	// first backup
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	testRunCheck(t, env.gopts)
	stat1 := dirStats(env.repo)

	// second backup, implicit incremental
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
	snapshotIDs := testListSnapshots(t, env.gopts, 2)

	stat2 := dirStats(env.repo)
	if stat2.size > stat1.size+stat1.size/10 {
		t.Error("repository size has grown by more than 10 percent")
	}
	t.Logf("repository grown by %d bytes", stat2.size-stat1.size)

	testRunCheck(t, env.gopts)
	// third backup, explicit incremental
	opts.Parent = snapshotIDs[0].String()
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
	snapshotIDs = testListSnapshots(t, env.gopts, 3)

	stat3 := dirStats(env.repo)
	if stat3.size > stat1.size+stat1.size/10 {
		t.Error("repository size has grown by more than 10 percent")
	}
	t.Logf("repository grown by %d bytes", stat3.size-stat2.size)

	// restore all backups and compare
	for i, snapshotID := range snapshotIDs {
		restoredir := filepath.Join(env.base, fmt.Sprintf("restore%d", i))
		t.Logf("restoring snapshot %v to %v", snapshotID.Str(), restoredir)
		testRunRestore(t, env.gopts, restoredir, snapshotID)
		diff := directoriesContentsDiff(env.testdata, filepath.Join(restoredir, "testdata"))
		rtest.Assert(t, diff == "", "directories are not equal: %v", diff)
	}

	testRunCheck(t, env.gopts)
}

func TestBackupWithRelativePath(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}

	// first backup
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
	firstSnapshotID := testListSnapshots(t, env.gopts, 1)[0]

	// second backup, implicit incremental
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)

	// that the correct parent snapshot was used
	latestSn, _ := testRunSnapshots(t, env.gopts)
	rtest.Assert(t, latestSn != nil, "missing latest snapshot")
	rtest.Assert(t, latestSn.Parent != nil && latestSn.Parent.Equal(firstSnapshotID), "second snapshot selected unexpected parent %v instead of %v", latestSn.Parent, firstSnapshotID)
}

func TestBackupParentSelection(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}

	// first backup
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata/0/0"}, opts, env.gopts)
	firstSnapshotID := testListSnapshots(t, env.gopts, 1)[0]

	// second backup, sibling path
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata/0/tests"}, opts, env.gopts)
	testListSnapshots(t, env.gopts, 2)

	// third backup, incremental for the first backup
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata/0/0"}, opts, env.gopts)

	// test that the correct parent snapshot was used
	latestSn, _ := testRunSnapshots(t, env.gopts)
	rtest.Assert(t, latestSn != nil, "missing latest snapshot")
	rtest.Assert(t, latestSn.Parent != nil && latestSn.Parent.Equal(firstSnapshotID), "third snapshot selected unexpected parent %v instead of %v", latestSn.Parent, firstSnapshotID)
}

func TestDryRunBackup(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	dryOpts := BackupOptions{DryRun: true}

	// dry run before first backup
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, dryOpts, env.gopts)
	snapshotIDs := testListSnapshots(t, env.gopts, 0)
	packIDs := testRunList(t, "packs", env.gopts)
	rtest.Assert(t, len(packIDs) == 0,
		"expected no data, got %v", snapshotIDs)
	indexIDs := testRunList(t, "index", env.gopts)
	rtest.Assert(t, len(indexIDs) == 0,
		"expected no index, got %v", snapshotIDs)

	// first backup
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
	snapshotIDs = testListSnapshots(t, env.gopts, 1)
	packIDs = testRunList(t, "packs", env.gopts)
	indexIDs = testRunList(t, "index", env.gopts)

	// dry run between backups
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, dryOpts, env.gopts)
	snapshotIDsAfter := testListSnapshots(t, env.gopts, 1)
	rtest.Equals(t, snapshotIDs, snapshotIDsAfter)
	dataIDsAfter := testRunList(t, "packs", env.gopts)
	rtest.Equals(t, packIDs, dataIDsAfter)
	indexIDsAfter := testRunList(t, "index", env.gopts)
	rtest.Equals(t, indexIDs, indexIDsAfter)

	// second backup, implicit incremental
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
	snapshotIDs = testListSnapshots(t, env.gopts, 2)
	packIDs = testRunList(t, "packs", env.gopts)
	indexIDs = testRunList(t, "index", env.gopts)

	// another dry run
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, dryOpts, env.gopts)
	snapshotIDsAfter = testListSnapshots(t, env.gopts, 2)
	rtest.Equals(t, snapshotIDs, snapshotIDsAfter)
	dataIDsAfter = testRunList(t, "packs", env.gopts)
	rtest.Equals(t, packIDs, dataIDsAfter)
	indexIDsAfter = testRunList(t, "index", env.gopts)
	rtest.Equals(t, indexIDs, indexIDsAfter)
}

func TestBackupNonExistingFile(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)

	_ = withRestoreGlobalOptions(func() error {
		globalOptions.stderr = io.Discard

		p := filepath.Join(env.testdata, "0", "0", "9")
		dirs := []string{
			filepath.Join(p, "0"),
			filepath.Join(p, "1"),
			filepath.Join(p, "nonexisting"),
			filepath.Join(p, "5"),
		}

		opts := BackupOptions{}

		testRunBackup(t, "", dirs, opts, env.gopts)
		return nil
	})
}

func TestBackupSelfHealing(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	p := filepath.Join(env.testdata, "test/test")
	rtest.OK(t, os.MkdirAll(filepath.Dir(p), 0755))
	rtest.OK(t, appendRandomData(p, 5))

	opts := BackupOptions{}

	testRunBackup(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	// remove all data packs
	removePacksExcept(env.gopts, t, restic.NewIDSet(), false)

	testRunRebuildIndex(t, env.gopts)
	// now the repo is also missing the data blob in the index; check should report this
	testRunCheckMustFail(t, env.gopts)

	// second backup should report an error but "heal" this situation
	err := testRunBackupAssumeFailure(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	rtest.Assert(t, err != nil,
		"backup should have reported an error")
	testRunCheck(t, env.gopts)
}

func TestBackupTreeLoadError(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)
	p := filepath.Join(env.testdata, "test/test")
	rtest.OK(t, os.MkdirAll(filepath.Dir(p), 0755))
	rtest.OK(t, appendRandomData(p, 5))

	opts := BackupOptions{}
	// Backup a subdirectory first, such that we can remove the tree pack for the subdirectory
	testRunBackup(t, env.testdata, []string{"test"}, opts, env.gopts)
	treePacks := listTreePacks(env.gopts, t)

	testRunBackup(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	// delete the subdirectory pack first
	removePacks(env.gopts, t, treePacks)
	testRunRebuildIndex(t, env.gopts)
	// now the repo is missing the tree blob in the index; check should report this
	testRunCheckMustFail(t, env.gopts)
	// second backup should report an error but "heal" this situation
	err := testRunBackupAssumeFailure(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	rtest.Assert(t, err != nil, "backup should have reported an error for the subdirectory")
	testRunCheck(t, env.gopts)

	// remove all tree packs
	removePacksExcept(env.gopts, t, restic.NewIDSet(), true)
	testRunRebuildIndex(t, env.gopts)
	// now the repo is also missing the data blob in the index; check should report this
	testRunCheckMustFail(t, env.gopts)
	// second backup should report an error but "heal" this situation
	err = testRunBackupAssumeFailure(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	rtest.Assert(t, err != nil, "backup should have reported an error")
	testRunCheck(t, env.gopts)
}

var backupExcludeFilenames = []string{
	"testfile1",
	"foo.tar.gz",
	"private/secret/passwords.txt",
	"work/source/test.c",
}

func TestBackupExclude(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	datadir := filepath.Join(env.base, "testdata")

	for _, filename := range backupExcludeFilenames {
		fp := filepath.Join(datadir, filename)
		rtest.OK(t, os.MkdirAll(filepath.Dir(fp), 0755))

		f, err := os.Create(fp)
		rtest.OK(t, err)

		fmt.Fprint(f, filename)
		rtest.OK(t, f.Close())
	}

	snapshots := make(map[string]struct{})

	opts := BackupOptions{}

	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
	snapshots, snapshotID := lastSnapshot(snapshots, loadSnapshotMap(t, env.gopts))
	files := testRunLs(t, env.gopts, snapshotID)
	rtest.Assert(t, includes(files, "/testdata/foo.tar.gz"),
		"expected file %q in first snapshot, but it's not included", "foo.tar.gz")

	opts.Excludes = []string{"*.tar.gz"}
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
	snapshots, snapshotID = lastSnapshot(snapshots, loadSnapshotMap(t, env.gopts))
	files = testRunLs(t, env.gopts, snapshotID)
	rtest.Assert(t, !includes(files, "/testdata/foo.tar.gz"),
		"expected file %q not in first snapshot, but it's included", "foo.tar.gz")

	opts.Excludes = []string{"*.tar.gz", "private/secret"}
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
	_, snapshotID = lastSnapshot(snapshots, loadSnapshotMap(t, env.gopts))
	files = testRunLs(t, env.gopts, snapshotID)
	rtest.Assert(t, !includes(files, "/testdata/foo.tar.gz"),
		"expected file %q not in first snapshot, but it's included", "foo.tar.gz")
	rtest.Assert(t, !includes(files, "/testdata/private/secret/passwords.txt"),
		"expected file %q not in first snapshot, but it's included", "passwords.txt")
}

func TestBackupErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		return
	}
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)

	// Assume failure
	inaccessibleFile := filepath.Join(env.testdata, "0", "0", "9", "0")
	rtest.OK(t, os.Chmod(inaccessibleFile, 0000))
	defer func() {
		rtest.OK(t, os.Chmod(inaccessibleFile, 0644))
	}()
	opts := BackupOptions{}
	err := testRunBackupAssumeFailure(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
	rtest.Assert(t, err != nil, "Assumed failure, but no error occurred.")
	rtest.Assert(t, err == ErrInvalidSourceData, "Wrong error returned")
	testListSnapshots(t, env.gopts, 1)
}

const (
	incrementalFirstWrite  = 10 * 1042 * 1024
	incrementalSecondWrite = 1 * 1042 * 1024
	incrementalThirdWrite  = 1 * 1042 * 1024
)

func TestIncrementalBackup(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	datadir := filepath.Join(env.base, "testdata")
	testfile := filepath.Join(datadir, "testfile")

	rtest.OK(t, appendRandomData(testfile, incrementalFirstWrite))

	opts := BackupOptions{}

	testRunBackup(t, "", []string{datadir}, opts, env.gopts)
	testRunCheck(t, env.gopts)
	stat1 := dirStats(env.repo)

	rtest.OK(t, appendRandomData(testfile, incrementalSecondWrite))

	testRunBackup(t, "", []string{datadir}, opts, env.gopts)
	testRunCheck(t, env.gopts)
	stat2 := dirStats(env.repo)
	if stat2.size-stat1.size > incrementalFirstWrite {
		t.Errorf("repository size has grown by more than %d bytes", incrementalFirstWrite)
	}
	t.Logf("repository grown by %d bytes", stat2.size-stat1.size)

	rtest.OK(t, appendRandomData(testfile, incrementalThirdWrite))

	testRunBackup(t, "", []string{datadir}, opts, env.gopts)
	testRunCheck(t, env.gopts)
	stat3 := dirStats(env.repo)
	if stat3.size-stat2.size > incrementalFirstWrite {
		t.Errorf("repository size has grown by more than %d bytes", incrementalFirstWrite)
	}
	t.Logf("repository grown by %d bytes", stat3.size-stat2.size)
}

// nolint: staticcheck // false positive nil pointer dereference check
func TestBackupTags(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}

	testRunBackup(t, "", []string{env.testdata}, opts, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ := testRunSnapshots(t, env.gopts)

	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}

	rtest.Assert(t, len(newest.Tags) == 0,
		"expected no tags, got %v", newest.Tags)
	parent := newest

	opts.Tags = restic.TagLists{[]string{"NL"}}
	testRunBackup(t, "", []string{env.testdata}, opts, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)

	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}

	rtest.Assert(t, len(newest.Tags) == 1 && newest.Tags[0] == "NL",
		"expected one NL tag, got %v", newest.Tags)
	// Tagged backup should have untagged backup as parent.
	rtest.Assert(t, parent.ID.Equal(*newest.Parent),
		"expected parent to be %v, got %v", parent.ID, newest.Parent)
}

// nolint: staticcheck // false positive nil pointer dereference check
func TestBackupProgramVersion(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	newest, _ := testRunSnapshots(t, env.gopts)

	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	resticVersion := "restic " + version
	rtest.Assert(t, newest.ProgramVersion == resticVersion,
		"expected %v, got %v", resticVersion, newest.ProgramVersion)
}

func TestQuietBackup(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}

	env.gopts.Quiet = false
	testRunBackup(t, "", []string{env.testdata}, opts, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	testRunCheck(t, env.gopts)

	env.gopts.Quiet = true
	testRunBackup(t, "", []string{env.testdata}, opts, env.gopts)
	testListSnapshots(t, env.gopts, 2)

	testRunCheck(t, env.gopts)
}

func TestHardLink(t *testing.T) {
	// this test assumes a test set with a single directory containing hard linked files
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("testdata", "test.hl.tar.gz")
	fd, err := os.Open(datafile)
	if os.IsNotExist(err) {
		t.Skipf("unable to find data file %q, skipping", datafile)
		return
	}
	rtest.OK(t, err)
	rtest.OK(t, fd.Close())

	testRunInit(t, env.gopts)

	rtest.SetupTarTestFixture(t, env.testdata, datafile)

	linkTests := createFileSetPerHardlink(env.testdata)

	opts := BackupOptions{}

	// first backup
	testRunBackup(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	snapshotIDs := testListSnapshots(t, env.gopts, 1)

	testRunCheck(t, env.gopts)

	// restore all backups and compare
	for i, snapshotID := range snapshotIDs {
		restoredir := filepath.Join(env.base, fmt.Sprintf("restore%d", i))
		t.Logf("restoring snapshot %v to %v", snapshotID.Str(), restoredir)
		testRunRestore(t, env.gopts, restoredir, snapshotID)
		diff := directoriesContentsDiff(env.testdata, filepath.Join(restoredir, "testdata"))
		rtest.Assert(t, diff == "", "directories are not equal %v", diff)

		linkResults := createFileSetPerHardlink(filepath.Join(restoredir, "testdata"))
		rtest.Assert(t, linksEqual(linkTests, linkResults),
			"links are not equal")
	}

	testRunCheck(t, env.gopts)
}

func linksEqual(source, dest map[uint64][]string) bool {
	for _, vs := range source {
		found := false
		for kd, vd := range dest {
			if linkEqual(vs, vd) {
				delete(dest, kd)
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return len(dest) == 0
}

func linkEqual(source, dest []string) bool {
	// equal if sliced are equal without considering order
	if source == nil && dest == nil {
		return true
	}

	if source == nil || dest == nil {
		return false
	}

	if len(source) != len(dest) {
		return false
	}

	for i := range source {
		found := false
		for j := range dest {
			if source[i] == dest[j] {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func TestStdinFromCommand(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{
		StdinCommand:  true,
		StdinFilename: "stdin",
	}

	testRunBackup(t, filepath.Dir(env.testdata), []string{"python", "-c", "import sys; print('something'); sys.exit(0)"}, opts, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	testRunCheck(t, env.gopts)
}

func TestStdinFromCommandNoOutput(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{
		StdinCommand:  true,
		StdinFilename: "stdin",
	}

	err := testRunBackupAssumeFailure(t, filepath.Dir(env.testdata), []string{"python", "-c", "import sys; sys.exit(0)"}, opts, env.gopts)
	rtest.Assert(t, err != nil && err.Error() == "at least one source file could not be read", "No data error expected")
	testListSnapshots(t, env.gopts, 1)

	testRunCheck(t, env.gopts)
}

func TestStdinFromCommandFailExitCode(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{
		StdinCommand:  true,
		StdinFilename: "stdin",
	}

	err := testRunBackupAssumeFailure(t, filepath.Dir(env.testdata), []string{"python", "-c", "import sys; print('test'); sys.exit(1)"}, opts, env.gopts)
	rtest.Assert(t, err != nil, "Expected error while backing up")

	testListSnapshots(t, env.gopts, 0)

	testRunCheck(t, env.gopts)
}

func TestStdinFromCommandFailNoOutputAndExitCode(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{
		StdinCommand:  true,
		StdinFilename: "stdin",
	}

	err := testRunBackupAssumeFailure(t, filepath.Dir(env.testdata), []string{"python", "-c", "import sys; sys.exit(1)"}, opts, env.gopts)
	rtest.Assert(t, err != nil, "Expected error while backing up")

	testListSnapshots(t, env.gopts, 0)

	testRunCheck(t, env.gopts)
}

func TestBackupEmptyPassword(t *testing.T) {
	// basic sanity test that empty passwords work
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	env.gopts.password = ""
	env.gopts.InsecureNoPassword = true

	testSetupBackupData(t, env)
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, BackupOptions{}, env.gopts)
	testListSnapshots(t, env.gopts, 1)
	testRunCheck(t, env.gopts)
}

func TestBackupSkipIfUnchanged(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{SkipIfUnchanged: true}

	for i := 0; i < 3; i++ {
		testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
		testListSnapshots(t, env.gopts, 1)
	}

	testRunCheck(t, env.gopts)
}
