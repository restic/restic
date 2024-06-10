package main

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/restic/restic/internal/feature"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/termstatus"
)

func testRunRestore(t testing.TB, opts GlobalOptions, dir string, snapshotID restic.ID) {
	testRunRestoreExcludes(t, opts, dir, snapshotID, nil)
}

func testRunRestoreExcludes(t testing.TB, gopts GlobalOptions, dir string, snapshotID restic.ID, excludes []string) {
	opts := RestoreOptions{
		Target: dir,
	}
	opts.Excludes = excludes

	rtest.OK(t, testRunRestoreAssumeFailure(snapshotID.String(), opts, gopts))
}

func testRunRestoreAssumeFailure(snapshotID string, opts RestoreOptions, gopts GlobalOptions) error {
	return withTermStatus(gopts, func(ctx context.Context, term *termstatus.Terminal) error {
		return runRestore(ctx, opts, gopts, term, []string{snapshotID})
	})
}

func testRunRestoreLatest(t testing.TB, gopts GlobalOptions, dir string, paths []string, hosts []string) {
	opts := RestoreOptions{
		Target: dir,
		SnapshotFilter: restic.SnapshotFilter{
			Hosts: hosts,
			Paths: paths,
		},
	}

	rtest.OK(t, testRunRestoreAssumeFailure("latest", opts, gopts))
}

func testRunRestoreIncludes(t testing.TB, gopts GlobalOptions, dir string, snapshotID restic.ID, includes []string) {
	opts := RestoreOptions{
		Target: dir,
	}
	opts.Includes = includes

	rtest.OK(t, testRunRestoreAssumeFailure(snapshotID.String(), opts, gopts))
}

func testRunRestoreIncludesFromFile(t testing.TB, gopts GlobalOptions, dir string, snapshotID restic.ID, includesFile string) {
	opts := RestoreOptions{
		Target: dir,
	}
	opts.IncludeFiles = []string{includesFile}

	rtest.OK(t, testRunRestoreAssumeFailure(snapshotID.String(), opts, gopts))
}

func testRunRestoreExcludesFromFile(t testing.TB, gopts GlobalOptions, dir string, snapshotID restic.ID, excludesFile string) {
	opts := RestoreOptions{
		Target: dir,
	}
	opts.ExcludeFiles = []string{excludesFile}

	rtest.OK(t, testRunRestoreAssumeFailure(snapshotID.String(), opts, gopts))
}

func TestRestoreMustFailWhenUsingBothIncludesAndExcludes(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	// Add both include and exclude patterns
	includePatterns := []string{"dir1/*include_me.txt", "dir2/**", "dir4/**/*_me.txt"}
	excludePatterns := []string{"dir1/*include_me.txt", "dir2/**", "dir4/**/*_me.txt"}

	restoredir := filepath.Join(env.base, "restore")

	restoreOpts := RestoreOptions{
		Target: restoredir,
	}
	restoreOpts.Includes = includePatterns
	restoreOpts.Excludes = excludePatterns

	err := testRunRestoreAssumeFailure("latest", restoreOpts, env.gopts)
	rtest.Assert(t, err != nil && strings.Contains(err.Error(), "exclude and include patterns are mutually exclusive"),
		"expected: %s error, got %v", "exclude and include patterns are mutually exclusive", err)
}

func TestRestoreIncludes(t *testing.T) {
	testfiles := []struct {
		path    string
		size    uint
		include bool // Whether this file should be included in the restore
	}{
		{"dir1/include_me.txt", 100, true},
		{"dir1/something_else.txt", 200, false},
		{"dir2/also_include_me.txt", 150, true},
		{"dir2/important_file.txt", 150, true},
		{"dir3/not_included.txt", 180, false},
		{"dir4/subdir/should_include_me.txt", 120, true},
	}

	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	// Create test files and directories
	for _, testFile := range testfiles {
		fullPath := filepath.Join(env.testdata, testFile.path)
		rtest.OK(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
		rtest.OK(t, appendRandomData(fullPath, testFile.size))
	}

	opts := BackupOptions{}

	// Perform backup
	testRunBackup(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	snapshotID := testListSnapshots(t, env.gopts, 1)[0]

	// Restore using includes
	includePatterns := []string{"dir1/*include_me.txt", "dir2/**", "dir4/**/*_me.txt"}
	restoredir := filepath.Join(env.base, "restore")
	testRunRestoreIncludes(t, env.gopts, restoredir, snapshotID, includePatterns)

	testRestoreFileInclusions := func(t *testing.T) {
		// Check that only the included files are restored
		for _, testFile := range testfiles {
			restoredFilePath := filepath.Join(restoredir, "testdata", testFile.path)
			_, err := os.Stat(restoredFilePath)
			if testFile.include {
				rtest.OK(t, err)
			} else {
				rtest.Assert(t, os.IsNotExist(err), "File %s should not have been restored", testFile.path)
			}
		}
	}

	testRestoreFileInclusions(t)

	// Create an include file with some patterns
	patternsFile := env.base + "/patternsFile"
	fileErr := os.WriteFile(patternsFile, []byte(strings.Join(includePatterns, "\n")), 0644)
	if fileErr != nil {
		t.Fatalf("Could not write include file: %v", fileErr)
	}

	restoredir = filepath.Join(env.base, "restore-include-from-file")

	testRunRestoreIncludesFromFile(t, env.gopts, restoredir, snapshotID, patternsFile)

	testRestoreFileInclusions(t)
}

func TestRestoreFilter(t *testing.T) {
	testfiles := []struct {
		name    string
		size    uint
		exclude bool
	}{
		{"testfile1.c", 100, true},
		{"testfile2.exe", 101, true},
		{"subdir1/subdir2/testfile3.docx", 102, true},
		{"subdir1/subdir2/testfile4.c", 102, false},
	}

	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	for _, testFile := range testfiles {
		p := filepath.Join(env.testdata, testFile.name)
		rtest.OK(t, os.MkdirAll(filepath.Dir(p), 0755))
		rtest.OK(t, appendRandomData(p, testFile.size))
	}

	opts := BackupOptions{}

	testRunBackup(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	snapshotID := testListSnapshots(t, env.gopts, 1)[0]

	// no restore filter should restore all files
	testRunRestore(t, env.gopts, filepath.Join(env.base, "restore0"), snapshotID)
	for _, testFile := range testfiles {
		rtest.OK(t, testFileSize(filepath.Join(env.base, "restore0", "testdata", testFile.name), int64(testFile.size)))
	}

	excludePatterns := []string{"testfile1.c", "*.exe", "*file3*"}

	// checks if the files are excluded correctly
	testRestoredFileExclusions := func(t *testing.T, restoredir string) {
		for _, testFile := range testfiles {
			restoredFilePath := filepath.Join(restoredir, "testdata", testFile.name)
			_, err := os.Stat(restoredFilePath)
			if testFile.exclude {
				rtest.Assert(t, os.IsNotExist(err), "File %s should not have been restored", testFile.name)
			} else {
				rtest.OK(t, testFileSize(restoredFilePath, int64(testFile.size)))
			}
		}
	}

	// restore with excludes
	restoredir := filepath.Join(env.base, "restore-with-excludes")
	testRunRestoreExcludes(t, env.gopts, restoredir, snapshotID, excludePatterns)
	testRestoredFileExclusions(t, restoredir)

	// Create an exclude file with some patterns
	patternsFile := env.base + "/patternsFile"
	fileErr := os.WriteFile(patternsFile, []byte(strings.Join(excludePatterns, "\n")), 0644)
	if fileErr != nil {
		t.Fatalf("Could not write include file: %v", fileErr)
	}

	// restore with excludes from file
	restoredir = filepath.Join(env.base, "restore-with-exclude-from-file")
	testRunRestoreExcludesFromFile(t, env.gopts, restoredir, snapshotID, patternsFile)

	testRestoredFileExclusions(t, restoredir)
}

func TestRestore(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	for i := 0; i < 10; i++ {
		p := filepath.Join(env.testdata, fmt.Sprintf("foo/bar/testfile%v", i))
		rtest.OK(t, os.MkdirAll(filepath.Dir(p), 0755))
		rtest.OK(t, appendRandomData(p, uint(rand.Intn(2<<21))))
	}

	opts := BackupOptions{}

	testRunBackup(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	// Restore latest without any filters
	restoredir := filepath.Join(env.base, "restore")
	testRunRestoreLatest(t, env.gopts, restoredir, nil, nil)

	diff := directoriesContentsDiff(env.testdata, filepath.Join(restoredir, filepath.Base(env.testdata)))
	rtest.Assert(t, diff == "", "directories are not equal %v", diff)
}

func TestRestoreLatest(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	p := filepath.Join(env.testdata, "testfile.c")
	rtest.OK(t, os.MkdirAll(filepath.Dir(p), 0755))
	rtest.OK(t, appendRandomData(p, 100))

	opts := BackupOptions{}

	// chdir manually here so we can get the current directory. This is not the
	// same as the temp dir returned by os.MkdirTemp() on darwin.
	back := rtest.Chdir(t, filepath.Dir(env.testdata))
	defer back()

	curdir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	testRunBackup(t, "", []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	rtest.OK(t, os.Remove(p))
	rtest.OK(t, appendRandomData(p, 101))
	testRunBackup(t, "", []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	// Restore latest without any filters
	testRunRestoreLatest(t, env.gopts, filepath.Join(env.base, "restore0"), nil, nil)
	rtest.OK(t, testFileSize(filepath.Join(env.base, "restore0", "testdata", "testfile.c"), int64(101)))

	// Setup test files in different directories backed up in different snapshots
	p1 := filepath.Join(curdir, filepath.FromSlash("p1/testfile.c"))

	rtest.OK(t, os.MkdirAll(filepath.Dir(p1), 0755))
	rtest.OK(t, appendRandomData(p1, 102))
	testRunBackup(t, "", []string{"p1"}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	p2 := filepath.Join(curdir, filepath.FromSlash("p2/testfile.c"))

	rtest.OK(t, os.MkdirAll(filepath.Dir(p2), 0755))
	rtest.OK(t, appendRandomData(p2, 103))
	testRunBackup(t, "", []string{"p2"}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	p1rAbs := filepath.Join(env.base, "restore1", "p1/testfile.c")
	p2rAbs := filepath.Join(env.base, "restore2", "p2/testfile.c")

	testRunRestoreLatest(t, env.gopts, filepath.Join(env.base, "restore1"), []string{filepath.Dir(p1)}, nil)
	rtest.OK(t, testFileSize(p1rAbs, int64(102)))
	if _, err := os.Stat(p2rAbs); os.IsNotExist(err) {
		rtest.Assert(t, os.IsNotExist(err),
			"expected %v to not exist in restore, but it exists, err %v", p2rAbs, err)
	}

	testRunRestoreLatest(t, env.gopts, filepath.Join(env.base, "restore2"), []string{filepath.Dir(p2)}, nil)
	rtest.OK(t, testFileSize(p2rAbs, int64(103)))
	if _, err := os.Stat(p1rAbs); os.IsNotExist(err) {
		rtest.Assert(t, os.IsNotExist(err),
			"expected %v to not exist in restore, but it exists, err %v", p1rAbs, err)
	}
}

func TestRestoreWithPermissionFailure(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("testdata", "repo-restore-permissions-test.tar.gz")
	rtest.SetupTarTestFixture(t, env.base, datafile)

	snapshots := testListSnapshots(t, env.gopts, 1)

	_ = withRestoreGlobalOptions(func() error {
		globalOptions.stderr = io.Discard
		testRunRestore(t, env.gopts, filepath.Join(env.base, "restore"), snapshots[0])
		return nil
	})

	// make sure that all files have been restored, regardless of any
	// permission errors
	files := testRunLs(t, env.gopts, snapshots[0].String())
	for _, filename := range files {
		fi, err := os.Lstat(filepath.Join(env.base, "restore", filename))
		rtest.OK(t, err)

		rtest.Assert(t, !isFile(fi) || fi.Size() > 0,
			"file %v restored, but filesize is 0", filename)
	}
}

func setZeroModTime(filename string) error {
	var utimes = []syscall.Timespec{
		syscall.NsecToTimespec(0),
		syscall.NsecToTimespec(0),
	}

	return syscall.UtimesNano(filename, utimes)
}

func TestRestoreNoMetadataOnIgnoredIntermediateDirs(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	p := filepath.Join(env.testdata, "subdir1", "subdir2", "subdir3", "file.ext")
	rtest.OK(t, os.MkdirAll(filepath.Dir(p), 0755))
	rtest.OK(t, appendRandomData(p, 200))
	rtest.OK(t, setZeroModTime(filepath.Join(env.testdata, "subdir1", "subdir2")))

	opts := BackupOptions{}

	testRunBackup(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	snapshotID := testListSnapshots(t, env.gopts, 1)[0]

	// restore with filter "*.ext", this should restore "file.ext", but
	// since the directories are ignored and only created because of
	// "file.ext", no meta data should be restored for them.
	testRunRestoreIncludes(t, env.gopts, filepath.Join(env.base, "restore0"), snapshotID, []string{"*.ext"})

	f1 := filepath.Join(env.base, "restore0", "testdata", "subdir1", "subdir2")
	_, err := os.Stat(f1)
	rtest.OK(t, err)

	// restore with filter "*", this should restore meta data on everything.
	testRunRestoreIncludes(t, env.gopts, filepath.Join(env.base, "restore1"), snapshotID, []string{"*"})

	f2 := filepath.Join(env.base, "restore1", "testdata", "subdir1", "subdir2")
	fi, err := os.Stat(f2)
	rtest.OK(t, err)

	rtest.Assert(t, fi.ModTime() == time.Unix(0, 0),
		"meta data of intermediate directory hasn't been restore")
}

func TestRestoreLocalLayout(t *testing.T) {
	defer feature.TestSetFlag(t, feature.Flag, feature.DeprecateS3LegacyLayout, false)()
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	var tests = []struct {
		filename string
		layout   string
	}{
		{"repo-layout-default.tar.gz", ""},
		{"repo-layout-s3legacy.tar.gz", ""},
		{"repo-layout-default.tar.gz", "default"},
		{"repo-layout-s3legacy.tar.gz", "s3legacy"},
	}

	for _, test := range tests {
		datafile := filepath.Join("..", "..", "internal", "backend", "testdata", test.filename)

		rtest.SetupTarTestFixture(t, env.base, datafile)

		env.gopts.extended["local.layout"] = test.layout

		// check the repo
		testRunCheck(t, env.gopts)

		// restore latest snapshot
		target := filepath.Join(env.base, "restore")
		testRunRestoreLatest(t, env.gopts, target, nil, nil)

		rtest.RemoveAll(t, filepath.Join(env.base, "repo"))
		rtest.RemoveAll(t, target)
	}
}
