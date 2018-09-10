package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	mrand "math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/fs"
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

	rtest.OK(t, runInit(opts, nil))
	t.Logf("repository initialized at %v", opts.Repo)
}

func testRunBackup(t testing.TB, dir string, target []string, opts BackupOptions, gopts GlobalOptions) {
	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()

	var wg errgroup.Group
	term := termstatus.New(gopts.stdout, gopts.stderr, gopts.Quiet)
	wg.Go(func() error { term.Run(ctx); return nil })

	gopts.stdout = ioutil.Discard
	t.Logf("backing up %v in %v", target, dir)
	if dir != "" {
		cleanup := fs.TestChdir(t, dir)
		defer cleanup()
	}

	rtest.OK(t, runBackup(opts, gopts, term, target))

	cancel()

	err := wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
}

func testRunList(t testing.TB, tpe string, opts GlobalOptions) restic.IDs {
	buf := bytes.NewBuffer(nil)
	globalOptions.stdout = buf
	defer func() {
		globalOptions.stdout = os.Stdout
	}()

	rtest.OK(t, runList(cmdList, opts, []string{tpe}))
	return parseIDsFromReader(t, buf)
}

func testRunRestore(t testing.TB, opts GlobalOptions, dir string, snapshotID restic.ID) {
	testRunRestoreExcludes(t, opts, dir, snapshotID, nil)
}

func testRunRestoreLatest(t testing.TB, gopts GlobalOptions, dir string, paths []string, host string) {
	opts := RestoreOptions{
		Target: dir,
		Host:   host,
		Paths:  paths,
	}

	rtest.OK(t, runRestore(opts, gopts, []string{"latest"}))
}

func testRunRestoreExcludes(t testing.TB, gopts GlobalOptions, dir string, snapshotID restic.ID, excludes []string) {
	opts := RestoreOptions{
		Target:  dir,
		Exclude: excludes,
	}

	rtest.OK(t, runRestore(opts, gopts, []string{snapshotID.String()}))
}

func testRunRestoreIncludes(t testing.TB, gopts GlobalOptions, dir string, snapshotID restic.ID, includes []string) {
	opts := RestoreOptions{
		Target:  dir,
		Include: includes,
	}

	rtest.OK(t, runRestore(opts, gopts, []string{snapshotID.String()}))
}

func testRunCheck(t testing.TB, gopts GlobalOptions) {
	opts := CheckOptions{
		ReadData:    true,
		CheckUnused: true,
	}
	rtest.OK(t, runCheck(opts, gopts, nil))
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

	err := runCheck(opts, gopts, nil)
	return string(buf.Bytes()), err
}

func testRunRebuildIndex(t testing.TB, gopts GlobalOptions) {
	globalOptions.stdout = ioutil.Discard
	defer func() {
		globalOptions.stdout = os.Stdout
	}()

	rtest.OK(t, runRebuildIndex(gopts))
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

	rtest.OK(t, runLs(opts, gopts, []string{snapshotID}))

	return strings.Split(string(buf.Bytes()), "\n")
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

	rtest.OK(t, runFind(opts, gopts, []string{pattern}))

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

	rtest.OK(t, runSnapshots(opts, globalOptions, []string{}))

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
	rtest.OK(t, runForget(opts, gopts, args))
}

func testRunPrune(t testing.TB, gopts GlobalOptions) {
	opts := PruneOptions{RepackThreshold: DefaultRepackThreshold}
	rtest.OK(t, runPrune(opts, gopts))
}

func TestBackup(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("testdata", "backup-data.tar.gz")
	fd, err := os.Open(datafile)
	if os.IsNotExist(errors.Cause(err)) {
		t.Skipf("unable to find data file %q, skipping", datafile)
		return
	}
	rtest.OK(t, err)
	rtest.OK(t, fd.Close())

	testRunInit(t, env.gopts)

	rtest.SetupTarTestFixture(t, env.testdata, datafile)
	opts := BackupOptions{}

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
		testRunRestore(t, env.gopts, restoredir, snapshotIDs[0])
		rtest.Assert(t, directoriesEqualContents(env.testdata, filepath.Join(restoredir, "testdata")),
			"directories are not equal")
	}

	testRunCheck(t, env.gopts)
}

func TestBackupNonExistingFile(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("testdata", "backup-data.tar.gz")
	fd, err := os.Open(datafile)
	if os.IsNotExist(errors.Cause(err)) {
		t.Skipf("unable to find data file %q, skipping", datafile)
		return
	}
	rtest.OK(t, err)
	rtest.OK(t, fd.Close())

	rtest.SetupTarTestFixture(t, env.testdata, datafile)

	testRunInit(t, env.gopts)
	globalOptions.stderr = ioutil.Discard
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

		fmt.Fprintf(f, filename)
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

	datafile := filepath.Join("testdata", "backup-data.tar.gz")
	testRunInit(t, env.gopts)
	rtest.SetupTarTestFixture(t, env.testdata, datafile)

	opts := BackupOptions{}

	testRunBackup(t, "", []string{env.testdata}, opts, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ := testRunSnapshots(t, env.gopts)
	rtest.Assert(t, newest != nil, "expected a new backup, got nil")
	rtest.Assert(t, len(newest.Tags) == 0,
		"expected no tags, got %v", newest.Tags)
	parent := newest

	opts.Tags = []string{"NL"}
	testRunBackup(t, "", []string{env.testdata}, opts, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	rtest.Assert(t, newest != nil, "expected a new backup, got nil")
	rtest.Assert(t, len(newest.Tags) == 1 && newest.Tags[0] == "NL",
		"expected one NL tag, got %v", newest.Tags)
	// Tagged backup should have untagged backup as parent.
	rtest.Assert(t, parent.ID.Equal(*newest.Parent),
		"expected parent to be %v, got %v", parent.ID, newest.Parent)
}

func testRunTag(t testing.TB, opts TagOptions, gopts GlobalOptions) {
	rtest.OK(t, runTag(opts, gopts, []string{}))
}

func TestTag(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("testdata", "backup-data.tar.gz")
	testRunInit(t, env.gopts)
	rtest.SetupTarTestFixture(t, env.testdata, datafile)

	testRunBackup(t, "", []string{env.testdata}, BackupOptions{}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ := testRunSnapshots(t, env.gopts)
	rtest.Assert(t, newest != nil, "expected a new backup, got nil")
	rtest.Assert(t, len(newest.Tags) == 0,
		"expected no tags, got %v", newest.Tags)
	rtest.Assert(t, newest.Original == nil,
		"expected original ID to be nil, got %v", newest.Original)
	originalID := *newest.ID

	testRunTag(t, TagOptions{SetTags: []string{"NL"}}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	rtest.Assert(t, newest != nil, "expected a new backup, got nil")
	rtest.Assert(t, len(newest.Tags) == 1 && newest.Tags[0] == "NL",
		"set failed, expected one NL tag, got %v", newest.Tags)
	rtest.Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
	rtest.Assert(t, *newest.Original == originalID,
		"expected original ID to be set to the first snapshot id")

	testRunTag(t, TagOptions{AddTags: []string{"CH"}}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	rtest.Assert(t, newest != nil, "expected a new backup, got nil")
	rtest.Assert(t, len(newest.Tags) == 2 && newest.Tags[0] == "NL" && newest.Tags[1] == "CH",
		"add failed, expected CH,NL tags, got %v", newest.Tags)
	rtest.Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
	rtest.Assert(t, *newest.Original == originalID,
		"expected original ID to be set to the first snapshot id")

	testRunTag(t, TagOptions{RemoveTags: []string{"NL"}}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	rtest.Assert(t, newest != nil, "expected a new backup, got nil")
	rtest.Assert(t, len(newest.Tags) == 1 && newest.Tags[0] == "CH",
		"remove failed, expected one CH tag, got %v", newest.Tags)
	rtest.Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
	rtest.Assert(t, *newest.Original == originalID,
		"expected original ID to be set to the first snapshot id")

	testRunTag(t, TagOptions{AddTags: []string{"US", "RU"}}, env.gopts)
	testRunTag(t, TagOptions{RemoveTags: []string{"CH", "US", "RU"}}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	rtest.Assert(t, newest != nil, "expected a new backup, got nil")
	rtest.Assert(t, len(newest.Tags) == 0,
		"expected no tags, got %v", newest.Tags)
	rtest.Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
	rtest.Assert(t, *newest.Original == originalID,
		"expected original ID to be set to the first snapshot id")

	// Check special case of removing all tags.
	testRunTag(t, TagOptions{SetTags: []string{""}}, env.gopts)
	testRunCheck(t, env.gopts)
	newest, _ = testRunSnapshots(t, env.gopts)
	rtest.Assert(t, newest != nil, "expected a new backup, got nil")
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

	rtest.OK(t, runKey(gopts, []string{"list"}))

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

	rtest.OK(t, runKey(gopts, []string{"add"}))
}

func testRunKeyPasswd(t testing.TB, newPassword string, gopts GlobalOptions) {
	testKeyNewPassword = newPassword
	defer func() {
		testKeyNewPassword = ""
	}()

	rtest.OK(t, runKey(gopts, []string{"passwd"}))
}

func testRunKeyRemove(t testing.TB, gopts GlobalOptions, IDs []string) {
	t.Logf("remove %d keys: %q\n", len(IDs), IDs)
	for _, id := range IDs {
		rtest.OK(t, runKey(gopts, []string{"remove", id}))
	}
}

func TestKeyAddRemove(t *testing.T) {
	passwordList := []string{
		"OnnyiasyatvodsEvVodyawit",
		"raicneirvOjEfEigonOmLasOd",
	}

	env, cleanup := withTestEnvironment(t)
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
	rtest.OK(t, runKey(env.gopts, []string{"list"}))
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
				rtest.Assert(t, os.IsNotExist(errors.Cause(err)),
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
	testRunRestoreLatest(t, env.gopts, restoredir, nil, "")

	rtest.Assert(t, directoriesEqualContents(env.testdata, filepath.Join(restoredir, filepath.Base(env.testdata))),
		"directories are not equal")
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
	// same as the temp dir returned by ioutil.TempDir() on darwin.
	back := fs.TestChdir(t, filepath.Dir(env.testdata))
	defer back()

	curdir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	testRunBackup(t, "", []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	os.Remove(p)
	rtest.OK(t, appendRandomData(p, 101))
	testRunBackup(t, "", []string{filepath.Base(env.testdata)}, opts, env.gopts)
	testRunCheck(t, env.gopts)

	// Restore latest without any filters
	testRunRestoreLatest(t, env.gopts, filepath.Join(env.base, "restore0"), nil, "")
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

	testRunRestoreLatest(t, env.gopts, filepath.Join(env.base, "restore1"), []string{filepath.Dir(p1)}, "")
	rtest.OK(t, testFileSize(p1rAbs, int64(102)))
	if _, err := os.Stat(p2rAbs); os.IsNotExist(errors.Cause(err)) {
		rtest.Assert(t, os.IsNotExist(errors.Cause(err)),
			"expected %v to not exist in restore, but it exists, err %v", p2rAbs, err)
	}

	testRunRestoreLatest(t, env.gopts, filepath.Join(env.base, "restore2"), []string{filepath.Dir(p2)}, "")
	rtest.OK(t, testFileSize(p2rAbs, int64(103)))
	if _, err := os.Stat(p1rAbs); os.IsNotExist(errors.Cause(err)) {
		rtest.Assert(t, os.IsNotExist(errors.Cause(err)),
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

	globalOptions.stderr = ioutil.Discard
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
	fi, err := os.Stat(f1)
	rtest.OK(t, err)

	// restore with filter "*", this should restore meta data on everything.
	testRunRestoreIncludes(t, env.gopts, filepath.Join(env.base, "restore1"), snapshotID, []string{"*"})

	f2 := filepath.Join(env.base, "restore1", "testdata", "subdir1", "subdir2")
	fi, err = os.Stat(f2)
	rtest.OK(t, err)

	rtest.Assert(t, fi.ModTime() == time.Unix(0, 0),
		"meta data of intermediate directory hasn't been restore")
}

func TestFind(t *testing.T) {
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("testdata", "backup-data.tar.gz")
	testRunInit(t, env.gopts)
	rtest.SetupTarTestFixture(t, env.testdata, datafile)

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

	datafile := filepath.Join("testdata", "backup-data.tar.gz")
	testRunInit(t, env.gopts)
	rtest.SetupTarTestFixture(t, env.testdata, datafile)

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

func TestRebuildIndex(t *testing.T) {
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

	testRunRebuildIndex(t, env.gopts)

	out, err = testRunCheckOutput(env.gopts)
	if len(out) != 0 {
		t.Fatalf("expected no output from the checker, got: %v", out)
	}

	if err != nil {
		t.Fatalf("expected no error from checker after rebuild-index, got: %v", err)
	}
}

func TestRebuildIndexAlwaysFull(t *testing.T) {
	repository.IndexFull = func(*repository.Index) bool { return true }
	TestRebuildIndex(t)
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
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("testdata", "backup-data.tar.gz")
	fd, err := os.Open(datafile)
	if os.IsNotExist(errors.Cause(err)) {
		t.Skipf("unable to find data file %q, skipping", datafile)
		return
	}
	rtest.OK(t, err)
	rtest.OK(t, fd.Close())

	testRunInit(t, env.gopts)

	rtest.SetupTarTestFixture(t, env.testdata, datafile)
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

	testRunForget(t, env.gopts, firstSnapshot[0].String())
	testRunPrune(t, env.gopts)
	testRunCheck(t, env.gopts)
}

func TestHardLink(t *testing.T) {
	// this test assumes a test set with a single directory containing hard linked files
	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := filepath.Join("testdata", "test.hl.tar.gz")
	fd, err := os.Open(datafile)
	if os.IsNotExist(errors.Cause(err)) {
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
		testRunRestore(t, env.gopts, restoredir, snapshotIDs[0])
		rtest.Assert(t, directoriesEqualContents(env.testdata, filepath.Join(restoredir, "testdata")),
			"directories are not equal")

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

	if len(dest) != 0 {
		return false
	}

	return true
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

	datafile := filepath.Join("testdata", "backup-data.tar.gz")
	fd, err := os.Open(datafile)
	if os.IsNotExist(errors.Cause(err)) {
		t.Skipf("unable to find data file %q, skipping", datafile)
		return
	}
	rtest.OK(t, err)
	rtest.OK(t, fd.Close())

	testRunInit(t, env.gopts)

	rtest.SetupTarTestFixture(t, env.testdata, datafile)
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
