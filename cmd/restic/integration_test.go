package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/index"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/termstatus"
	"golang.org/x/sync/errgroup"
)

func parseIDsFromReader(t testing.TB, rd io.Reader) restic.IDs {
	IDs := restic.IDs{}
	sc := bufio.NewScanner(rd)

	for sc.Scan() {
		id, err := restic.ParseID(sc.Text())
		if err != nil {
			t.Logf("parse id %v: %v", sc.Text(), err)
			continue
		}

		IDs = append(IDs, id)
	}

	return IDs
}

func testRunInit(t testing.TB, opts GlobalOptions) {
	repository.TestUseLowSecurityKDFParameters(t)
	restic.TestDisableCheckPolynomial(t)
	restic.TestSetLockTimeout(t, 0)

	rtest.OK(t, runInit(context.TODO(), InitOptions{}, opts, nil))
	t.Logf("repository initialized at %v", opts.Repo)
}

func testRunBackupAssumeFailure(t testing.TB, dir string, target []string, opts BackupOptions, gopts GlobalOptions) error {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	var wg errgroup.Group
	term := termstatus.New(gopts.stdout, gopts.stderr, gopts.Quiet)
	wg.Go(func() error { term.Run(ctx); return nil })

	gopts.stdout = io.Discard
	t.Logf("backing up %v in %v", target, dir)
	if dir != "" {
		cleanup := rtest.Chdir(t, dir)
		defer cleanup()
	}

	backupErr := runBackup(ctx, opts, gopts, term, target)

	cancel()

	err := wg.Wait()
	if err != nil {
		t.Fatal(err)
	}

	return backupErr
}

func testRunBackup(t testing.TB, dir string, target []string, opts BackupOptions, gopts GlobalOptions) {
	err := testRunBackupAssumeFailure(t, dir, target, opts, gopts)
	rtest.Assert(t, err == nil, "Error while backing up")
}

func testRunList(t testing.TB, tpe string, opts GlobalOptions) restic.IDs {
	buf := bytes.NewBuffer(nil)
	globalOptions.stdout = buf
	defer func() {
		globalOptions.stdout = os.Stdout
	}()

	rtest.OK(t, runList(context.TODO(), cmdList, opts, []string{tpe}))
	return parseIDsFromReader(t, buf)
}

func testRunRestore(t testing.TB, opts GlobalOptions, dir string, snapshotID restic.ID) {
	testRunRestoreExcludes(t, opts, dir, snapshotID, nil)
}

func testRunRestoreLatest(t testing.TB, gopts GlobalOptions, dir string, paths []string, hosts []string) {
	opts := RestoreOptions{
		Target: dir,
		SnapshotFilter: restic.SnapshotFilter{
			Hosts: hosts,
			Paths: paths,
		},
	}

	rtest.OK(t, runRestore(context.TODO(), opts, gopts, []string{"latest"}))
}

func testRunRestoreExcludes(t testing.TB, gopts GlobalOptions, dir string, snapshotID restic.ID, excludes []string) {
	opts := RestoreOptions{
		Target:  dir,
		Exclude: excludes,
	}

	rtest.OK(t, runRestore(context.TODO(), opts, gopts, []string{snapshotID.String()}))
}

func testRunRestoreIncludes(t testing.TB, gopts GlobalOptions, dir string, snapshotID restic.ID, includes []string) {
	opts := RestoreOptions{
		Target:  dir,
		Include: includes,
	}

	rtest.OK(t, runRestore(context.TODO(), opts, gopts, []string{snapshotID.String()}))
}

func testRunRestoreAssumeFailure(t testing.TB, snapshotID string, opts RestoreOptions, gopts GlobalOptions) error {
	err := runRestore(context.TODO(), opts, gopts, []string{snapshotID})

	return err
}

func testRunCheck(t testing.TB, gopts GlobalOptions) {
	opts := CheckOptions{
		ReadData:    true,
		CheckUnused: true,
	}
	rtest.OK(t, runCheck(context.TODO(), opts, gopts, nil))
}

func testRunCheckOutput(gopts GlobalOptions) (string, error) {
	buf := bytes.NewBuffer(nil)

	globalOptions.stdout = buf
	defer func() {
		globalOptions.stdout = os.Stdout
	}()

	opts := CheckOptions{
		ReadData: true,
	}

	err := runCheck(context.TODO(), opts, gopts, nil)
	return buf.String(), err
}

func testRunDiffOutput(gopts GlobalOptions, firstSnapshotID string, secondSnapshotID string) (string, error) {
	buf := bytes.NewBuffer(nil)

	globalOptions.stdout = buf
	oldStdout := gopts.stdout
	gopts.stdout = buf
	defer func() {
		globalOptions.stdout = os.Stdout
		gopts.stdout = oldStdout
	}()

	opts := DiffOptions{
		ShowMetadata: false,
	}
	err := runDiff(context.TODO(), opts, gopts, []string{firstSnapshotID, secondSnapshotID})
	return buf.String(), err
}

func testRunRebuildIndex(t testing.TB, gopts GlobalOptions) {
	globalOptions.stdout = io.Discard
	defer func() {
		globalOptions.stdout = os.Stdout
	}()

	rtest.OK(t, runRebuildIndex(context.TODO(), RebuildIndexOptions{}, gopts))
}

func testRunLs(t testing.TB, gopts GlobalOptions, snapshotID string) []string {
	buf := bytes.NewBuffer(nil)
	globalOptions.stdout = buf
	quiet := globalOptions.Quiet
	globalOptions.Quiet = true
	defer func() {
		globalOptions.stdout = os.Stdout
		globalOptions.Quiet = quiet
	}()

	opts := LsOptions{}

	rtest.OK(t, runLs(context.TODO(), opts, gopts, []string{snapshotID}))

	return strings.Split(buf.String(), "\n")
}

func testRunFind(t testing.TB, wantJSON bool, gopts GlobalOptions, pattern string) []byte {
	buf := bytes.NewBuffer(nil)
	globalOptions.stdout = buf
	globalOptions.JSON = wantJSON
	defer func() {
		globalOptions.stdout = os.Stdout
		globalOptions.JSON = false
	}()

	opts := FindOptions{}

	rtest.OK(t, runFind(context.TODO(), opts, gopts, []string{pattern}))

	return buf.Bytes()
}

func testRunSnapshots(t testing.TB, gopts GlobalOptions) (newest *Snapshot, snapmap map[restic.ID]Snapshot) {
	buf := bytes.NewBuffer(nil)
	globalOptions.stdout = buf
	globalOptions.JSON = true
	defer func() {
		globalOptions.stdout = os.Stdout
		globalOptions.JSON = gopts.JSON
	}()

	opts := SnapshotOptions{}

	rtest.OK(t, runSnapshots(context.TODO(), opts, globalOptions, []string{}))

	snapshots := []Snapshot{}
	rtest.OK(t, json.Unmarshal(buf.Bytes(), &snapshots))

	snapmap = make(map[restic.ID]Snapshot, len(snapshots))
	for _, sn := range snapshots {
		snapmap[*sn.ID] = sn
		if newest == nil || sn.Time.After(newest.Time) {
			newest = &sn
		}
	}
	return
}

func testRunForget(t testing.TB, gopts GlobalOptions, args ...string) {
	opts := ForgetOptions{}
	rtest.OK(t, runForget(context.TODO(), opts, gopts, args))
}

func testRunForgetJSON(t testing.TB, gopts GlobalOptions, args ...string) {
	buf := bytes.NewBuffer(nil)
	oldJSON := gopts.JSON
	gopts.stdout = buf
	gopts.JSON = true
	defer func() {
		gopts.stdout = os.Stdout
		gopts.JSON = oldJSON
	}()

	opts := ForgetOptions{
		DryRun: true,
		Last:   1,
	}

	rtest.OK(t, runForget(context.TODO(), opts, gopts, args))

	var forgets []*ForgetGroup
	rtest.OK(t, json.Unmarshal(buf.Bytes(), &forgets))

	rtest.Assert(t, len(forgets) == 1,
		"Expected 1 snapshot group, got %v", len(forgets))
	rtest.Assert(t, len(forgets[0].Keep) == 1,
		"Expected 1 snapshot to be kept, got %v", len(forgets[0].Keep))
	rtest.Assert(t, len(forgets[0].Remove) == 2,
		"Expected 2 snapshots to be removed, got %v", len(forgets[0].Remove))
}

func testRunPrune(t testing.TB, gopts GlobalOptions, opts PruneOptions) {
	oldHook := gopts.backendTestHook
	gopts.backendTestHook = func(r restic.Backend) (restic.Backend, error) { return newListOnceBackend(r), nil }
	defer func() {
		gopts.backendTestHook = oldHook
	}()
	rtest.OK(t, runPrune(context.TODO(), opts, gopts))
}

func testSetupBackupData(t testing.TB, env *testEnvironment) string {
	datafile := filepath.Join("testdata", "backup-data.tar.gz")
	testRunInit(t, env.gopts)
	rtest.SetupTarTestFixture(t, env.testdata, datafile)
	return datafile
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
	snapshotIDs := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 1,
		"expected one snapshot, got %v", snapshotIDs)

	testRunCheck(t, env.gopts)
	stat1 := dirStats(env.repo)

	// second backup, implicit incremental
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
	snapshotIDs = testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 2,
		"expected two snapshots, got %v", snapshotIDs)

	stat2 := dirStats(env.repo)
	if stat2.size > stat1.size+stat1.size/10 {
		t.Error("repository size has grown by more than 10 percent")
	}
	t.Logf("repository grown by %d bytes", stat2.size-stat1.size)

	testRunCheck(t, env.gopts)
	// third backup, explicit incremental
	opts.Parent = snapshotIDs[0].String()
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
	snapshotIDs = testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 3,
		"expected three snapshots, got %v", snapshotIDs)

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

func TestDryRunBackup(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	dryOpts := BackupOptions{DryRun: true}

	// dry run before first backup
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, dryOpts, env.gopts)
	snapshotIDs := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 0,
		"expected no snapshot, got %v", snapshotIDs)
	packIDs := testRunList(t, "packs", env.gopts)
	rtest.Assert(t, len(packIDs) == 0,
		"expected no data, got %v", snapshotIDs)
	indexIDs := testRunList(t, "index", env.gopts)
	rtest.Assert(t, len(indexIDs) == 0,
		"expected no index, got %v", snapshotIDs)

	// first backup
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
	snapshotIDs = testRunList(t, "snapshots", env.gopts)
	packIDs = testRunList(t, "packs", env.gopts)
	indexIDs = testRunList(t, "index", env.gopts)

	// dry run between backups
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, dryOpts, env.gopts)
	snapshotIDsAfter := testRunList(t, "snapshots", env.gopts)
	rtest.Equals(t, snapshotIDs, snapshotIDsAfter)
	dataIDsAfter := testRunList(t, "packs", env.gopts)
	rtest.Equals(t, packIDs, dataIDsAfter)
	indexIDsAfter := testRunList(t, "index", env.gopts)
	rtest.Equals(t, indexIDs, indexIDsAfter)

	// second backup, implicit incremental
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, env.gopts)
	snapshotIDs = testRunList(t, "snapshots", env.gopts)
	packIDs = testRunList(t, "packs", env.gopts)
	indexIDs = testRunList(t, "index", env.gopts)

	// another dry run
	testRunBackup(t, filepath.Dir(env.testdata), []string{"testdata"}, dryOpts, env.gopts)
	snapshotIDsAfter = testRunList(t, "snapshots", env.gopts)
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
	globalOptions.stderr = io.Discard
	defer func() {
		globalOptions.stderr = os.Stderr
	}()

	p := filepath.Join(env.testdata, "0", "0", "9")
	dirs := []string{
		filepath.Join(p, "0"),
		filepath.Join(p, "1"),
		filepath.Join(p, "nonexisting"),
		filepath.Join(p, "5"),
	}

	opts := BackupOptions{}

	testRunBackup(t, "", dirs, opts, env.gopts)
}

func removePacksExcept(gopts GlobalOptions, t *testing.T, keep restic.IDSet, removeTreePacks bool) {
	r, err := OpenRepository(context.TODO(), gopts)
	rtest.OK(t, err)

	// Get all tree packs
	rtest.OK(t, r.LoadIndex(context.TODO()))

	treePacks := restic.NewIDSet()
	r.Index().Each(context.TODO(), func(pb restic.PackedBlob) {
		if pb.Type == restic.TreeBlob {
			treePacks.Insert(pb.PackID)
		}
	})

	// remove all packs containing data blobs
	rtest.OK(t, r.List(context.TODO(), restic.PackFile, func(id restic.ID, size int64) error {
		if treePacks.Has(id) != removeTreePacks || keep.Has(id) {
			return nil
		}
		return r.Backend().Remove(context.TODO(), restic.Handle{Type: restic.PackFile, Name: id.String()})
	}))
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
	rtest.Assert(t, runCheck(context.TODO(), CheckOptions{}, env.gopts, nil) != nil,
		"check should have reported an error")

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

	r, err := OpenRepository(context.TODO(), env.gopts)
	rtest.OK(t, err)
	rtest.OK(t, r.LoadIndex(context.TODO()))
	treePacks := restic.NewIDSet()
	r.Index().Each(context.TODO(), func(pb restic.PackedBlob) {
		if pb.Type == restic.TreeBlob {
			treePacks.Insert(pb.PackID)
		}
	})

	testRunBackup(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	// delete the subdirectory pack first
	for id := range treePacks {
		rtest.OK(t, r.Backend().Remove(context.TODO(), restic.Handle{Type: restic.PackFile, Name: id.String()}))
	}
	testRunRebuildIndex(t, env.gopts)
	// now the repo is missing the tree blob in the index; check should report this
	rtest.Assert(t, runCheck(context.TODO(), CheckOptions{}, env.gopts, nil) != nil, "check should have reported an error")
	// second backup should report an error but "heal" this situation
	err = testRunBackupAssumeFailure(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	rtest.Assert(t, err != nil, "backup should have reported an error for the subdirectory")
	testRunCheck(t, env.gopts)

	// remove all tree packs
	removePacksExcept(env.gopts, t, restic.NewIDSet(), true)
	testRunRebuildIndex(t, env.gopts)
	// now the repo is also missing the data blob in the index; check should report this
	rtest.Assert(t, runCheck(context.TODO(), CheckOptions{}, env.gopts, nil) != nil, "check should have reported an error")
	// second backup should report an error but "heal" this situation
	err = testRunBackupAssumeFailure(t, filepath.Dir(env.testdata), []string{filepath.Base(env.testdata)}, opts, env.gopts)
	rtest.Assert(t, err != nil, "backup should have reported an error")
	testRunCheck(t, env.gopts)
}

func includes(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}

	return false
}

func loadSnapshotMap(t testing.TB, gopts GlobalOptions) map[string]struct{} {
	snapshotIDs := testRunList(t, "snapshots", gopts)

	m := make(map[string]struct{})
	for _, id := range snapshotIDs {
		m[id.String()] = struct{}{}
	}

	return m
}

func lastSnapshot(old, new map[string]struct{}) (map[string]struct{}, string) {
	for k := range new {
		if _, ok := old[k]; !ok {
			old[k] = struct{}{}
			return old, k
		}
	}

	return old, ""
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
	gopts := env.gopts
	gopts.stderr = io.Discard
	err := testRunBackupAssumeFailure(t, filepath.Dir(env.testdata), []string{"testdata"}, opts, gopts)
	rtest.Assert(t, err != nil, "Assumed failure, but no error occurred.")
	rtest.Assert(t, err == ErrInvalidSourceData, "Wrong error returned")
	snapshotIDs := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 1,
		"expected one snapshot, got %v", snapshotIDs)
}

const (
	incrementalFirstWrite  = 10 * 1042 * 1024
	incrementalSecondWrite = 1 * 1042 * 1024
	incrementalThirdWrite  = 1 * 1042 * 1024
)

func appendRandomData(filename string, bytes uint) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		return err
	}

	_, err = f.Seek(0, 2)
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		return err
	}

	_, err = io.Copy(f, io.LimitReader(rand.Reader, int64(bytes)))
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		return err
	}

	return f.Close()
}

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

func testRunCopy(t testing.TB, srcGopts GlobalOptions, dstGopts GlobalOptions) {
	gopts := srcGopts
	gopts.Repo = dstGopts.Repo
	gopts.password = dstGopts.password
	copyOpts := CopyOptions{
		secondaryRepoOptions: secondaryRepoOptions{
			Repo:     srcGopts.Repo,
			password: srcGopts.password,
		},
	}

	rtest.OK(t, runCopy(context.TODO(), copyOpts, gopts, nil))
}

func TestCopy(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, opts, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "3")}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	testRunInit(t, env2.gopts)
	testRunCopy(t, env.gopts, env2.gopts)

	snapshotIDs := testRunList(t, "snapshots", env.gopts)
	copiedSnapshotIDs := testRunList(t, "snapshots", env2.gopts)

	// Check that the copies size seems reasonable
	rtest.Assert(t, len(snapshotIDs) == len(copiedSnapshotIDs), "expected %v snapshots, found %v",
		len(snapshotIDs), len(copiedSnapshotIDs))
	stat := dirStats(env.repo)
	stat2 := dirStats(env2.repo)
	sizeDiff := int64(stat.size) - int64(stat2.size)
	if sizeDiff < 0 {
		sizeDiff = -sizeDiff
	}
	rtest.Assert(t, sizeDiff < int64(stat.size)/50, "expected less than 2%% size difference: %v vs. %v",
		stat.size, stat2.size)

	// Check integrity of the copy
	testRunCheck(t, env2.gopts)

	// Check that the copied snapshots have the same tree contents as the old ones (= identical tree hash)
	origRestores := make(map[string]struct{})
	for i, snapshotID := range snapshotIDs {
		restoredir := filepath.Join(env.base, fmt.Sprintf("restore%d", i))
		origRestores[restoredir] = struct{}{}
		testRunRestore(t, env.gopts, restoredir, snapshotID)
	}
	for i, snapshotID := range copiedSnapshotIDs {
		restoredir := filepath.Join(env2.base, fmt.Sprintf("restore%d", i))
		testRunRestore(t, env2.gopts, restoredir, snapshotID)
		foundMatch := false
		for cmpdir := range origRestores {
			diff := directoriesContentsDiff(restoredir, cmpdir)
			if diff == "" {
				delete(origRestores, cmpdir)
				foundMatch = true
			}
		}

		rtest.Assert(t, foundMatch, "found no counterpart for snapshot %v", snapshotID)
	}

	rtest.Assert(t, len(origRestores) == 0, "found not copied snapshots")
}

func TestCopyIncremental(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()

	testSetupBackupData(t, env)
	opts := BackupOptions{}
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	testRunInit(t, env2.gopts)
	testRunCopy(t, env.gopts, env2.gopts)

	snapshotIDs := testRunList(t, "snapshots", env.gopts)
	copiedSnapshotIDs := testRunList(t, "snapshots", env2.gopts)

	// Check that the copies size seems reasonable
	testRunCheck(t, env2.gopts)
	rtest.Assert(t, len(snapshotIDs) == len(copiedSnapshotIDs), "expected %v snapshots, found %v",
		len(snapshotIDs), len(copiedSnapshotIDs))

	// check that no snapshots are copied, as there are no new ones
	testRunCopy(t, env.gopts, env2.gopts)
	testRunCheck(t, env2.gopts)
	copiedSnapshotIDs = testRunList(t, "snapshots", env2.gopts)
	rtest.Assert(t, len(snapshotIDs) == len(copiedSnapshotIDs), "still expected %v snapshots, found %v",
		len(snapshotIDs), len(copiedSnapshotIDs))

	// check that only new snapshots are copied
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "3")}, opts, env.gopts)
	testRunCopy(t, env.gopts, env2.gopts)
	testRunCheck(t, env2.gopts)
	snapshotIDs = testRunList(t, "snapshots", env.gopts)
	copiedSnapshotIDs = testRunList(t, "snapshots", env2.gopts)
	rtest.Assert(t, len(snapshotIDs) == len(copiedSnapshotIDs), "still expected %v snapshots, found %v",
		len(snapshotIDs), len(copiedSnapshotIDs))

	// also test the reverse direction
	testRunCopy(t, env2.gopts, env.gopts)
	testRunCheck(t, env.gopts)
	snapshotIDs = testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == len(copiedSnapshotIDs), "still expected %v snapshots, found %v",
		len(copiedSnapshotIDs), len(snapshotIDs))
}

func TestCopyUnstableJSON(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()

	// contains a symlink created using `ln -s '../i/'$'\355\246\361''d/samba' broken-symlink`
	datafile := filepath.Join("testdata", "copy-unstable-json.tar.gz")
	rtest.SetupTarTestFixture(t, env.base, datafile)

	testRunInit(t, env2.gopts)
	testRunCopy(t, env.gopts, env2.gopts)
	testRunCheck(t, env2.gopts)

	copiedSnapshotIDs := testRunList(t, "snapshots", env2.gopts)
	rtest.Assert(t, 1 == len(copiedSnapshotIDs), "still expected %v snapshot, found %v",
		1, len(copiedSnapshotIDs))
}

func TestInitCopyChunkerParams(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()
	env2, cleanup2 := withTestEnvironment(t)
	defer cleanup2()

	testRunInit(t, env2.gopts)

	initOpts := InitOptions{
		secondaryRepoOptions: secondaryRepoOptions{
			Repo:     env2.gopts.Repo,
			password: env2.gopts.password,
		},
	}
	rtest.Assert(t, runInit(context.TODO(), initOpts, env.gopts, nil) != nil, "expected invalid init options to fail")

	initOpts.CopyChunkerParameters = true
	rtest.OK(t, runInit(context.TODO(), initOpts, env.gopts, nil))

	repo, err := OpenRepository(context.TODO(), env.gopts)
	rtest.OK(t, err)

	otherRepo, err := OpenRepository(context.TODO(), env2.gopts)
	rtest.OK(t, err)

	rtest.Assert(t, repo.Config().ChunkerPolynomial == otherRepo.Config().ChunkerPolynomial,
		"expected equal chunker polynomials, got %v expected %v", repo.Config().ChunkerPolynomial,
		otherRepo.Config().ChunkerPolynomial)
}

func testRunTag(t testing.TB, opts TagOptions, gopts GlobalOptions) {
	rtest.OK(t, runTag(context.TODO(), opts, gopts, []string{}))
}

func TestTag(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ := testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a new backup, got nil")
	}

	rtest.Assert(t, len(newest.Tags) == 0,
		"expected no tags, got %v", newest.Tags)
	rtest.Assert(t, newest.Original == nil,
		"expected original ID to be nil, got %v", newest.Original)
	originalID := *newest.ID

	testRunTag(t, TagOptions{SetTags: restic.TagLists{[]string{"NL"}}}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, len(newest.Tags) == 1 && newest.Tags[0] == "NL",
		"set failed, expected one NL tag, got %v", newest.Tags)
	rtest.Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
	rtest.Assert(t, *newest.Original == originalID,
		"expected original ID to be set to the first snapshot id")

	testRunTag(t, TagOptions{AddTags: restic.TagLists{[]string{"CH"}}}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, len(newest.Tags) == 2 && newest.Tags[0] == "NL" && newest.Tags[1] == "CH",
		"add failed, expected CH,NL tags, got %v", newest.Tags)
	rtest.Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
	rtest.Assert(t, *newest.Original == originalID,
		"expected original ID to be set to the first snapshot id")

	testRunTag(t, TagOptions{RemoveTags: restic.TagLists{[]string{"NL"}}}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, len(newest.Tags) == 1 && newest.Tags[0] == "CH",
		"remove failed, expected one CH tag, got %v", newest.Tags)
	rtest.Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
	rtest.Assert(t, *newest.Original == originalID,
		"expected original ID to be set to the first snapshot id")

	testRunTag(t, TagOptions{AddTags: restic.TagLists{[]string{"US", "RU"}}}, env.gopts)
	testRunTag(t, TagOptions{RemoveTags: restic.TagLists{[]string{"CH", "US", "RU"}}}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, len(newest.Tags) == 0,
		"expected no tags, got %v", newest.Tags)
	rtest.Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
	rtest.Assert(t, *newest.Original == originalID,
		"expected original ID to be set to the first snapshot id")

	// Check special case of removing all tags.
	testRunTag(t, TagOptions{SetTags: restic.TagLists{[]string{""}}}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	if newest == nil {
		t.Fatal("expected a backup, got nil")
	}
	rtest.Assert(t, len(newest.Tags) == 0,
		"expected no tags, got %v", newest.Tags)
	rtest.Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
	rtest.Assert(t, *newest.Original == originalID,
		"expected original ID to be set to the first snapshot id")
}

func testRunKeyListOtherIDs(t testing.TB, gopts GlobalOptions) []string {
	buf := bytes.NewBuffer(nil)

	globalOptions.stdout = buf
	defer func() {
		globalOptions.stdout = os.Stdout
	}()

	rtest.OK(t, runKey(context.TODO(), gopts, []string{"list"}))

	scanner := bufio.NewScanner(buf)
	exp := regexp.MustCompile(`^ ([a-f0-9]+) `)

	IDs := []string{}
	for scanner.Scan() {
		if id := exp.FindStringSubmatch(scanner.Text()); id != nil {
			IDs = append(IDs, id[1])
		}
	}

	return IDs
}

func testRunKeyAddNewKey(t testing.TB, newPassword string, gopts GlobalOptions) {
	testKeyNewPassword = newPassword
	defer func() {
		testKeyNewPassword = ""
	}()

	rtest.OK(t, runKey(context.TODO(), gopts, []string{"add"}))
}

func testRunKeyAddNewKeyUserHost(t testing.TB, gopts GlobalOptions) {
	testKeyNewPassword = "john's geheimnis"
	defer func() {
		testKeyNewPassword = ""
		keyUsername = ""
		keyHostname = ""
	}()

	rtest.OK(t, cmdKey.Flags().Parse([]string{"--user=john", "--host=example.com"}))

	t.Log("adding key for john@example.com")
	rtest.OK(t, runKey(context.TODO(), gopts, []string{"add"}))

	repo, err := OpenRepository(context.TODO(), gopts)
	rtest.OK(t, err)
	key, err := repository.SearchKey(context.TODO(), repo, testKeyNewPassword, 2, "")
	rtest.OK(t, err)

	rtest.Equals(t, "john", key.Username)
	rtest.Equals(t, "example.com", key.Hostname)
}

func testRunKeyPasswd(t testing.TB, newPassword string, gopts GlobalOptions) {
	testKeyNewPassword = newPassword
	defer func() {
		testKeyNewPassword = ""
	}()

	rtest.OK(t, runKey(context.TODO(), gopts, []string{"passwd"}))
}

func testRunKeyRemove(t testing.TB, gopts GlobalOptions, IDs []string) {
	t.Logf("remove %d keys: %q\n", len(IDs), IDs)
	for _, id := range IDs {
		rtest.OK(t, runKey(context.TODO(), gopts, []string{"remove", id}))
	}
}

func TestKeyAddRemove(t *testing.T) {
	passwordList := []string{
		"OnnyiasyatvodsEvVodyawit",
		"raicneirvOjEfEigonOmLasOd",
	}

	env, cleanup := withTestEnvironment(t)
	// must list keys more than once
	env.gopts.backendTestHook = nil
	defer cleanup()

	testRunInit(t, env.gopts)

	testRunKeyPasswd(t, "geheim2", env.gopts)
	env.gopts.password = "geheim2"
	t.Logf("changed password to %q", env.gopts.password)

	for _, newPassword := range passwordList {
		testRunKeyAddNewKey(t, newPassword, env.gopts)
		t.Logf("added new password %q", newPassword)
		env.gopts.password = newPassword
		testRunKeyRemove(t, env.gopts, testRunKeyListOtherIDs(t, env.gopts))
	}

	env.gopts.password = passwordList[len(passwordList)-1]
	t.Logf("testing access with last password %q\n", env.gopts.password)
	rtest.OK(t, runKey(context.TODO(), env.gopts, []string{"list"}))
	testRunCheck(t, env.gopts)

	testRunKeyAddNewKeyUserHost(t, env.gopts)
}

type emptySaveBackend struct {
	restic.Backend
}

func (b *emptySaveBackend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	return b.Backend.Save(ctx, h, restic.NewByteReader([]byte{}, nil))
}

func TestKeyProblems(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)
	env.gopts.backendTestHook = func(r restic.Backend) (restic.Backend, error) {
		return &emptySaveBackend{r}, nil
	}

	testKeyNewPassword = "geheim2"
	defer func() {
		testKeyNewPassword = ""
	}()

	err := runKey(context.TODO(), env.gopts, []string{"passwd"})
	t.Log(err)
	rtest.Assert(t, err != nil, "expected passwd change to fail")

	err = runKey(context.TODO(), env.gopts, []string{"add"})
	t.Log(err)
	rtest.Assert(t, err != nil, "expected key adding to fail")

	t.Logf("testing access with initial password %q\n", env.gopts.password)
	rtest.OK(t, runKey(context.TODO(), env.gopts, []string{"list"}))
	testRunCheck(t, env.gopts)
}

func testFileSize(filename string, size int64) error {
	fi, err := os.Stat(filename)
	if err != nil {
		return err
	}

	if fi.Size() != size {
		return errors.Fatalf("wrong file size for %v: expected %v, got %v", filename, size, fi.Size())
	}

	return nil
}

func TestRestoreFilter(t *testing.T) {
	testfiles := []struct {
		name string
		size uint
	}{
		{"testfile1.c", 100},
		{"testfile2.exe", 101},
		{"subdir1/subdir2/testfile3.docx", 102},
		{"subdir1/subdir2/testfile4.c", 102},
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

	snapshotID := testRunList(t, "snapshots", env.gopts)[0]

	// no restore filter should restore all files
	testRunRestore(t, env.gopts, filepath.Join(env.base, "restore0"), snapshotID)
	for _, testFile := range testfiles {
		rtest.OK(t, testFileSize(filepath.Join(env.base, "restore0", "testdata", testFile.name), int64(testFile.size)))
	}

	for i, pat := range []string{"*.c", "*.exe", "*", "*file3*"} {
		base := filepath.Join(env.base, fmt.Sprintf("restore%d", i+1))
		testRunRestoreExcludes(t, env.gopts, base, snapshotID, []string{pat})
		for _, testFile := range testfiles {
			err := testFileSize(filepath.Join(base, "testdata", testFile.name), int64(testFile.size))
			if ok, _ := filter.Match(pat, filepath.Base(testFile.name)); !ok {
				rtest.OK(t, err)
			} else {
				rtest.Assert(t, os.IsNotExist(err),
					"expected %v to not exist in restore step %v, but it exists, err %v", testFile.name, i+1, err)
			}
		}
	}
}

func TestRestore(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testRunInit(t, env.gopts)

	for i := 0; i < 10; i++ {
		p := filepath.Join(env.testdata, fmt.Sprintf("foo/bar/testfile%v", i))
		rtest.OK(t, os.MkdirAll(filepath.Dir(p), 0755))
		rtest.OK(t, appendRandomData(p, uint(mrand.Intn(2<<21))))
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

	snapshots := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshots) > 0,
		"no snapshots found in repo (%v)", datafile)

	globalOptions.stderr = io.Discard
	defer func() {
		globalOptions.stderr = os.Stderr
	}()

	testRunRestore(t, env.gopts, filepath.Join(env.base, "restore"), snapshots[0])

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

	snapshotID := testRunList(t, "snapshots", env.gopts)[0]

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

func TestFind(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := testSetupBackupData(t, env)
	opts := BackupOptions{}

	testRunBackup(t, "", []string{env.testdata}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	results := testRunFind(t, false, env.gopts, "unexistingfile")
	rtest.Assert(t, len(results) == 0, "unexisting file found in repo (%v)", datafile)

	results = testRunFind(t, false, env.gopts, "testfile")
	lines := strings.Split(string(results), "\n")
	rtest.Assert(t, len(lines) == 2, "expected one file found in repo (%v)", datafile)

	results = testRunFind(t, false, env.gopts, "testfile*")
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

	results := testRunFind(t, true, env.gopts, "unexistingfile")
	matches := []testMatches{}
	rtest.OK(t, json.Unmarshal(results, &matches))
	rtest.Assert(t, len(matches) == 0, "expected no match in repo (%v)", datafile)

	results = testRunFind(t, true, env.gopts, "testfile")
	rtest.OK(t, json.Unmarshal(results, &matches))
	rtest.Assert(t, len(matches) == 1, "expected a single snapshot in repo (%v)", datafile)
	rtest.Assert(t, len(matches[0].Matches) == 1, "expected a single file to match (%v)", datafile)
	rtest.Assert(t, matches[0].Hits == 1, "expected hits to show 1 match (%v)", datafile)

	results = testRunFind(t, true, env.gopts, "testfile*")
	rtest.OK(t, json.Unmarshal(results, &matches))
	rtest.Assert(t, len(matches) == 1, "expected a single snapshot in repo (%v)", datafile)
	rtest.Assert(t, len(matches[0].Matches) == 3, "expected 3 files to match (%v)", datafile)
	rtest.Assert(t, matches[0].Hits == 3, "expected hits to show 3 matches (%v)", datafile)
}

func testRebuildIndex(t *testing.T, backendTestHook backendWrapper) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("..", "..", "internal", "checker", "testdata", "duplicate-packs-in-index-test-repo.tar.gz")
	rtest.SetupTarTestFixture(t, env.base, datafile)

	out, err := testRunCheckOutput(env.gopts)
	if !strings.Contains(out, "contained in several indexes") {
		t.Fatalf("did not find checker hint for packs in several indexes")
	}

	if err != nil {
		t.Fatalf("expected no error from checker for test repository, got %v", err)
	}

	if !strings.Contains(out, "restic rebuild-index") {
		t.Fatalf("did not find hint for rebuild-index command")
	}

	env.gopts.backendTestHook = backendTestHook
	testRunRebuildIndex(t, env.gopts)

	env.gopts.backendTestHook = nil
	out, err = testRunCheckOutput(env.gopts)
	if len(out) != 0 {
		t.Fatalf("expected no output from the checker, got: %v", out)
	}

	if err != nil {
		t.Fatalf("expected no error from checker after rebuild-index, got: %v", err)
	}
}

func TestRebuildIndex(t *testing.T) {
	testRebuildIndex(t, nil)
}

func TestRebuildIndexAlwaysFull(t *testing.T) {
	indexFull := index.IndexFull
	defer func() {
		index.IndexFull = indexFull
	}()
	index.IndexFull = func(*index.Index, bool) bool { return true }
	testRebuildIndex(t, nil)
}

// indexErrorBackend modifies the first index after reading.
type indexErrorBackend struct {
	restic.Backend
	lock     sync.Mutex
	hasErred bool
}

func (b *indexErrorBackend) Load(ctx context.Context, h restic.Handle, length int, offset int64, consumer func(rd io.Reader) error) error {
	return b.Backend.Load(ctx, h, length, offset, func(rd io.Reader) error {
		// protect hasErred
		b.lock.Lock()
		defer b.lock.Unlock()
		if !b.hasErred && h.Type == restic.IndexFile {
			b.hasErred = true
			return consumer(errorReadCloser{rd})
		}
		return consumer(rd)
	})
}

type errorReadCloser struct {
	io.Reader
}

func (erd errorReadCloser) Read(p []byte) (int, error) {
	n, err := erd.Reader.Read(p)
	if n > 0 {
		p[0] ^= 1
	}
	return n, err
}

func TestRebuildIndexDamage(t *testing.T) {
	testRebuildIndex(t, func(r restic.Backend) (restic.Backend, error) {
		return &indexErrorBackend{
			Backend: r,
		}, nil
	})
}

type appendOnlyBackend struct {
	restic.Backend
}

// called via repo.Backend().Remove()
func (b *appendOnlyBackend) Remove(ctx context.Context, h restic.Handle) error {
	return errors.Errorf("Failed to remove %v", h)
}

func TestRebuildIndexFailsOnAppendOnly(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("..", "..", "internal", "checker", "testdata", "duplicate-packs-in-index-test-repo.tar.gz")
	rtest.SetupTarTestFixture(t, env.base, datafile)

	globalOptions.stdout = io.Discard
	defer func() {
		globalOptions.stdout = os.Stdout
	}()

	env.gopts.backendTestHook = func(r restic.Backend) (restic.Backend, error) {
		return &appendOnlyBackend{r}, nil
	}
	err := runRebuildIndex(context.TODO(), RebuildIndexOptions{}, env.gopts)
	if err == nil {
		t.Error("expected rebuildIndex to fail")
	}
	t.Log(err)
}

func TestCheckRestoreNoLock(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("testdata", "small-repo.tar.gz")
	rtest.SetupTarTestFixture(t, env.base, datafile)

	err := filepath.Walk(env.repo, func(p string, fi os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		return os.Chmod(p, fi.Mode() & ^(os.FileMode(0222)))
	})
	rtest.OK(t, err)

	env.gopts.NoLock = true

	testRunCheck(t, env.gopts)

	snapshotIDs := testRunList(t, "snapshots", env.gopts)
	if len(snapshotIDs) == 0 {
		t.Fatalf("found no snapshots")
	}

	testRunRestore(t, env.gopts, filepath.Join(env.base, "restore"), snapshotIDs[0])
}

func TestPrune(t *testing.T) {
	testPruneVariants(t, false)
	testPruneVariants(t, true)
}

func testPruneVariants(t *testing.T, unsafeNoSpaceRecovery bool) {
	suffix := ""
	if unsafeNoSpaceRecovery {
		suffix = "-recovery"
	}
	t.Run("0"+suffix, func(t *testing.T) {
		opts := PruneOptions{MaxUnused: "0%", unsafeRecovery: unsafeNoSpaceRecovery}
		checkOpts := CheckOptions{ReadData: true, CheckUnused: true}
		testPrune(t, opts, checkOpts)
	})

	t.Run("50"+suffix, func(t *testing.T) {
		opts := PruneOptions{MaxUnused: "50%", unsafeRecovery: unsafeNoSpaceRecovery}
		checkOpts := CheckOptions{ReadData: true}
		testPrune(t, opts, checkOpts)
	})

	t.Run("unlimited"+suffix, func(t *testing.T) {
		opts := PruneOptions{MaxUnused: "unlimited", unsafeRecovery: unsafeNoSpaceRecovery}
		checkOpts := CheckOptions{ReadData: true}
		testPrune(t, opts, checkOpts)
	})

	t.Run("CachableOnly"+suffix, func(t *testing.T) {
		opts := PruneOptions{MaxUnused: "5%", RepackCachableOnly: true, unsafeRecovery: unsafeNoSpaceRecovery}
		checkOpts := CheckOptions{ReadData: true}
		testPrune(t, opts, checkOpts)
	})
	t.Run("Small", func(t *testing.T) {
		opts := PruneOptions{MaxUnused: "unlimited", RepackSmall: true}
		checkOpts := CheckOptions{ReadData: true, CheckUnused: true}
		testPrune(t, opts, checkOpts)
	})
}

func createPrunableRepo(t *testing.T, env *testEnvironment) {
	testSetupBackupData(t, env)
	opts := BackupOptions{}

	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)
	firstSnapshot := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(firstSnapshot) == 1,
		"expected one snapshot, got %v", firstSnapshot)

	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, opts, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "3")}, opts, env.gopts)

	snapshotIDs := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 3,
		"expected 3 snapshot, got %v", snapshotIDs)

	testRunForgetJSON(t, env.gopts)
	testRunForget(t, env.gopts, firstSnapshot[0].String())
}

func testPrune(t *testing.T, pruneOpts PruneOptions, checkOpts CheckOptions) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	createPrunableRepo(t, env)
	testRunPrune(t, env.gopts, pruneOpts)
	rtest.OK(t, runCheck(context.TODO(), checkOpts, env.gopts, nil))
}

var pruneDefaultOptions = PruneOptions{MaxUnused: "5%"}

func listPacks(gopts GlobalOptions, t *testing.T) restic.IDSet {
	r, err := OpenRepository(context.TODO(), gopts)
	rtest.OK(t, err)

	packs := restic.NewIDSet()

	rtest.OK(t, r.List(context.TODO(), restic.PackFile, func(id restic.ID, size int64) error {
		packs.Insert(id)
		return nil
	}))
	return packs
}

func TestPruneWithDamagedRepository(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("testdata", "backup-data.tar.gz")
	testRunInit(t, env.gopts)

	rtest.SetupTarTestFixture(t, env.testdata, datafile)
	opts := BackupOptions{}

	// create and delete snapshot to create unused blobs
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, opts, env.gopts)
	firstSnapshot := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(firstSnapshot) == 1,
		"expected one snapshot, got %v", firstSnapshot)
	testRunForget(t, env.gopts, firstSnapshot[0].String())

	oldPacks := listPacks(env.gopts, t)

	// create new snapshot, but lose all data
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "3")}, opts, env.gopts)
	snapshotIDs := testRunList(t, "snapshots", env.gopts)

	removePacksExcept(env.gopts, t, oldPacks, false)

	rtest.Assert(t, len(snapshotIDs) == 1,
		"expected one snapshot, got %v", snapshotIDs)

	oldHook := env.gopts.backendTestHook
	env.gopts.backendTestHook = func(r restic.Backend) (restic.Backend, error) { return newListOnceBackend(r), nil }
	defer func() {
		env.gopts.backendTestHook = oldHook
	}()
	// prune should fail
	rtest.Assert(t, runPrune(context.TODO(), pruneDefaultOptions, env.gopts) == errorPacksMissing,
		"prune should have reported index not complete error")
}

// Test repos for edge cases
func TestEdgeCaseRepos(t *testing.T) {
	opts := CheckOptions{}

	// repo where index is completely missing
	// => check and prune should fail
	t.Run("no-index", func(t *testing.T) {
		testEdgeCaseRepo(t, "repo-index-missing.tar.gz", opts, pruneDefaultOptions, false, false)
	})

	// repo where an existing and used blob is missing from the index
	// => check and prune should fail
	t.Run("index-missing-blob", func(t *testing.T) {
		testEdgeCaseRepo(t, "repo-index-missing-blob.tar.gz", opts, pruneDefaultOptions, false, false)
	})

	// repo where a blob is missing
	// => check and prune should fail
	t.Run("missing-data", func(t *testing.T) {
		testEdgeCaseRepo(t, "repo-data-missing.tar.gz", opts, pruneDefaultOptions, false, false)
	})

	// repo where blobs which are not needed are missing or in invalid pack files
	// => check should fail and prune should repair this
	t.Run("missing-unused-data", func(t *testing.T) {
		testEdgeCaseRepo(t, "repo-unused-data-missing.tar.gz", opts, pruneDefaultOptions, false, true)
	})

	// repo where data exists that is not referenced
	// => check and prune should fully work
	t.Run("unreferenced-data", func(t *testing.T) {
		testEdgeCaseRepo(t, "repo-unreferenced-data.tar.gz", opts, pruneDefaultOptions, true, true)
	})

	// repo where an obsolete index still exists
	// => check and prune should fully work
	t.Run("obsolete-index", func(t *testing.T) {
		testEdgeCaseRepo(t, "repo-obsolete-index.tar.gz", opts, pruneDefaultOptions, true, true)
	})

	// repo which contains mixed (data/tree) packs
	// => check and prune should fully work
	t.Run("mixed-packs", func(t *testing.T) {
		testEdgeCaseRepo(t, "repo-mixed.tar.gz", opts, pruneDefaultOptions, true, true)
	})

	// repo which contains duplicate blobs
	// => checking for unused data should report an error and prune resolves the
	// situation
	opts = CheckOptions{
		ReadData:    true,
		CheckUnused: true,
	}
	t.Run("duplicates", func(t *testing.T) {
		testEdgeCaseRepo(t, "repo-duplicates.tar.gz", opts, pruneDefaultOptions, false, true)
	})
}

func testEdgeCaseRepo(t *testing.T, tarfile string, optionsCheck CheckOptions, optionsPrune PruneOptions, checkOK, pruneOK bool) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("testdata", tarfile)
	rtest.SetupTarTestFixture(t, env.base, datafile)

	if checkOK {
		testRunCheck(t, env.gopts)
	} else {
		rtest.Assert(t, runCheck(context.TODO(), optionsCheck, env.gopts, nil) != nil,
			"check should have reported an error")
	}

	if pruneOK {
		testRunPrune(t, env.gopts, optionsPrune)
		testRunCheck(t, env.gopts)
	} else {
		rtest.Assert(t, runPrune(context.TODO(), optionsPrune, env.gopts) != nil,
			"prune should have reported an error")
	}
}

// a listOnceBackend only allows listing once per filetype
// listing filetypes more than once may cause problems with eventually consistent
// backends (like e.g. Amazon S3) as the second listing may be inconsistent to what
// is expected by the first listing + some operations.
type listOnceBackend struct {
	restic.Backend
	listedFileType map[restic.FileType]bool
	strictOrder    bool
}

func newListOnceBackend(be restic.Backend) *listOnceBackend {
	return &listOnceBackend{
		Backend:        be,
		listedFileType: make(map[restic.FileType]bool),
		strictOrder:    false,
	}
}

func newOrderedListOnceBackend(be restic.Backend) *listOnceBackend {
	return &listOnceBackend{
		Backend:        be,
		listedFileType: make(map[restic.FileType]bool),
		strictOrder:    true,
	}
}

func (be *listOnceBackend) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
	if t != restic.LockFile && be.listedFileType[t] {
		return errors.Errorf("tried listing type %v the second time", t)
	}
	if be.strictOrder && t == restic.SnapshotFile && be.listedFileType[restic.IndexFile] {
		return errors.Errorf("tried listing type snapshots after index")
	}
	be.listedFileType[t] = true
	return be.Backend.List(ctx, t, fn)
}

func TestListOnce(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	env.gopts.backendTestHook = func(r restic.Backend) (restic.Backend, error) {
		return newListOnceBackend(r), nil
	}
	pruneOpts := PruneOptions{MaxUnused: "0"}
	checkOpts := CheckOptions{ReadData: true, CheckUnused: true}

	createPrunableRepo(t, env)
	testRunPrune(t, env.gopts, pruneOpts)
	rtest.OK(t, runCheck(context.TODO(), checkOpts, env.gopts, nil))

	rtest.OK(t, runRebuildIndex(context.TODO(), RebuildIndexOptions{}, env.gopts))
	rtest.OK(t, runRebuildIndex(context.TODO(), RebuildIndexOptions{ReadAllPacks: true}, env.gopts))
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
	snapshotIDs := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 1,
		"expected one snapshot, got %v", snapshotIDs)

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

func TestQuietBackup(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	testSetupBackupData(t, env)
	opts := BackupOptions{}

	env.gopts.Quiet = false
	testRunBackup(t, "", []string{env.testdata}, opts, env.gopts)
	snapshotIDs := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 1,
		"expected one snapshot, got %v", snapshotIDs)

	testRunCheck(t, env.gopts)

	env.gopts.Quiet = true
	testRunBackup(t, "", []string{env.testdata}, opts, env.gopts)
	snapshotIDs = testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(snapshotIDs) == 2,
		"expected two snapshots, got %v", snapshotIDs)

	testRunCheck(t, env.gopts)
}

func copyFile(dst string, src string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		// ignore subsequent errors
		_ = srcFile.Close()
		return err
	}

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		// ignore subsequent errors
		_ = srcFile.Close()
		_ = dstFile.Close()
		return err
	}

	err = srcFile.Close()
	if err != nil {
		// ignore subsequent errors
		_ = dstFile.Close()
		return err
	}

	err = dstFile.Close()
	if err != nil {
		return err
	}

	return nil
}

var diffOutputRegexPatterns = []string{
	"-.+modfile",
	"M.+modfile1",
	"\\+.+modfile2",
	"\\+.+modfile3",
	"\\+.+modfile4",
	"-.+submoddir",
	"-.+submoddir.subsubmoddir",
	"\\+.+submoddir2",
	"\\+.+submoddir2.subsubmoddir",
	"Files: +2 new, +1 removed, +1 changed",
	"Dirs: +3 new, +2 removed",
	"Data Blobs: +2 new, +1 removed",
	"Added: +7[0-9]{2}\\.[0-9]{3} KiB",
	"Removed: +2[0-9]{2}\\.[0-9]{3} KiB",
}

func setupDiffRepo(t *testing.T) (*testEnvironment, func(), string, string) {
	env, cleanup := withTestEnvironment(t)
	testRunInit(t, env.gopts)

	datadir := filepath.Join(env.base, "testdata")
	testdir := filepath.Join(datadir, "testdir")
	subtestdir := filepath.Join(testdir, "subtestdir")
	testfile := filepath.Join(testdir, "testfile")

	rtest.OK(t, os.Mkdir(testdir, 0755))
	rtest.OK(t, os.Mkdir(subtestdir, 0755))
	rtest.OK(t, appendRandomData(testfile, 256*1024))

	moddir := filepath.Join(datadir, "moddir")
	submoddir := filepath.Join(moddir, "submoddir")
	subsubmoddir := filepath.Join(submoddir, "subsubmoddir")
	modfile := filepath.Join(moddir, "modfile")
	rtest.OK(t, os.Mkdir(moddir, 0755))
	rtest.OK(t, os.Mkdir(submoddir, 0755))
	rtest.OK(t, os.Mkdir(subsubmoddir, 0755))
	rtest.OK(t, copyFile(modfile, testfile))
	rtest.OK(t, appendRandomData(modfile+"1", 256*1024))

	snapshots := make(map[string]struct{})
	opts := BackupOptions{}
	testRunBackup(t, "", []string{datadir}, opts, env.gopts)
	snapshots, firstSnapshotID := lastSnapshot(snapshots, loadSnapshotMap(t, env.gopts))

	rtest.OK(t, os.Rename(modfile, modfile+"3"))
	rtest.OK(t, os.Rename(submoddir, submoddir+"2"))
	rtest.OK(t, appendRandomData(modfile+"1", 256*1024))
	rtest.OK(t, appendRandomData(modfile+"2", 256*1024))
	rtest.OK(t, os.Mkdir(modfile+"4", 0755))

	testRunBackup(t, "", []string{datadir}, opts, env.gopts)
	_, secondSnapshotID := lastSnapshot(snapshots, loadSnapshotMap(t, env.gopts))

	return env, cleanup, firstSnapshotID, secondSnapshotID
}

func TestDiff(t *testing.T) {
	env, cleanup, firstSnapshotID, secondSnapshotID := setupDiffRepo(t)
	defer cleanup()

	// quiet suppresses the diff output except for the summary
	env.gopts.Quiet = false
	_, err := testRunDiffOutput(env.gopts, "", secondSnapshotID)
	rtest.Assert(t, err != nil, "expected error on invalid snapshot id")

	out, err := testRunDiffOutput(env.gopts, firstSnapshotID, secondSnapshotID)
	rtest.OK(t, err)

	for _, pattern := range diffOutputRegexPatterns {
		r, err := regexp.Compile(pattern)
		rtest.Assert(t, err == nil, "failed to compile regexp %v", pattern)
		rtest.Assert(t, r.MatchString(out), "expected pattern %v in output, got\n%v", pattern, out)
	}

	// check quiet output
	env.gopts.Quiet = true
	outQuiet, err := testRunDiffOutput(env.gopts, firstSnapshotID, secondSnapshotID)
	rtest.OK(t, err)

	rtest.Assert(t, len(outQuiet) < len(out), "expected shorter output on quiet mode %v vs. %v", len(outQuiet), len(out))
}

type typeSniffer struct {
	MessageType string `json:"message_type"`
}

func TestDiffJSON(t *testing.T) {
	env, cleanup, firstSnapshotID, secondSnapshotID := setupDiffRepo(t)
	defer cleanup()

	// quiet suppresses the diff output except for the summary
	env.gopts.Quiet = false
	env.gopts.JSON = true
	out, err := testRunDiffOutput(env.gopts, firstSnapshotID, secondSnapshotID)
	rtest.OK(t, err)

	var stat DiffStatsContainer
	var changes int

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		var sniffer typeSniffer
		rtest.OK(t, json.Unmarshal([]byte(line), &sniffer))
		switch sniffer.MessageType {
		case "change":
			changes++
		case "statistics":
			rtest.OK(t, json.Unmarshal([]byte(line), &stat))
		default:
			t.Fatalf("unexpected message type %v", sniffer.MessageType)
		}
	}
	rtest.Equals(t, 9, changes)
	rtest.Assert(t, stat.Added.Files == 2 && stat.Added.Dirs == 3 && stat.Added.DataBlobs == 2 &&
		stat.Removed.Files == 1 && stat.Removed.Dirs == 2 && stat.Removed.DataBlobs == 1 &&
		stat.ChangedFiles == 1, "unexpected statistics")

	// check quiet output
	env.gopts.Quiet = true
	outQuiet, err := testRunDiffOutput(env.gopts, firstSnapshotID, secondSnapshotID)
	rtest.OK(t, err)

	stat = DiffStatsContainer{}
	rtest.OK(t, json.Unmarshal([]byte(outQuiet), &stat))
	rtest.Assert(t, stat.Added.Files == 2 && stat.Added.Dirs == 3 && stat.Added.DataBlobs == 2 &&
		stat.Removed.Files == 1 && stat.Removed.Dirs == 2 && stat.Removed.DataBlobs == 1 &&
		stat.ChangedFiles == 1, "unexpected statistics")
	rtest.Assert(t, stat.SourceSnapshot == firstSnapshotID && stat.TargetSnapshot == secondSnapshotID, "unexpected snapshot ids")
}

type writeToOnly struct {
	rd io.Reader
}

func (r *writeToOnly) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("should have called WriteTo instead")
}

func (r *writeToOnly) WriteTo(w io.Writer) (int64, error) {
	return io.Copy(w, r.rd)
}

type onlyLoadWithWriteToBackend struct {
	restic.Backend
}

func (be *onlyLoadWithWriteToBackend) Load(ctx context.Context, h restic.Handle,
	length int, offset int64, fn func(rd io.Reader) error) error {

	return be.Backend.Load(ctx, h, length, offset, func(rd io.Reader) error {
		return fn(&writeToOnly{rd: rd})
	})
}

func TestBackendLoadWriteTo(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	// setup backend which only works if it's WriteTo method is correctly propagated upwards
	env.gopts.backendInnerTestHook = func(r restic.Backend) (restic.Backend, error) {
		return &onlyLoadWithWriteToBackend{Backend: r}, nil
	}

	testSetupBackupData(t, env)

	// add some data, but make sure that it isn't cached during upload
	opts := BackupOptions{}
	env.gopts.NoCache = true
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)

	// loading snapshots must still work
	env.gopts.NoCache = false
	firstSnapshot := testRunList(t, "snapshots", env.gopts)
	rtest.Assert(t, len(firstSnapshot) == 1,
		"expected one snapshot, got %v", firstSnapshot)
}

func TestFindListOnce(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	env.gopts.backendTestHook = func(r restic.Backend) (restic.Backend, error) {
		return newListOnceBackend(r), nil
	}

	testSetupBackupData(t, env)
	opts := BackupOptions{}

	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9")}, opts, env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "2")}, opts, env.gopts)
	secondSnapshot := testRunList(t, "snapshots", env.gopts)
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "9", "3")}, opts, env.gopts)
	thirdSnapshot := restic.NewIDSet(testRunList(t, "snapshots", env.gopts)...)

	repo, err := OpenRepository(context.TODO(), env.gopts)
	rtest.OK(t, err)

	snapshotIDs := restic.NewIDSet()
	// specify the two oldest snapshots explicitly and use "latest" to reference the newest one
	for sn := range FindFilteredSnapshots(context.TODO(), repo.Backend(), repo, &restic.SnapshotFilter{}, []string{
		secondSnapshot[0].String(),
		secondSnapshot[1].String()[:8],
		"latest",
	}) {
		snapshotIDs.Insert(*sn.ID())
	}

	// the snapshots can only be listed once, if both lists match then the there has been only a single List() call
	rtest.Equals(t, thirdSnapshot, snapshotIDs)
}
