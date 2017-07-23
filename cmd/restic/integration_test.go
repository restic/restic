package main

import (
	"bufio"
	"bytes"
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

	"github.com/restic/restic/internal"

	"github.com/restic/restic/internal/errors"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/repository"
	. "github.com/restic/restic/internal/test"
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
	restic.TestSetLockTimeout(t, 0)

	OK(t, runInit(opts, nil))
	t.Logf("repository initialized at %v", opts.Repo)
}

func testRunBackup(t testing.TB, target []string, opts BackupOptions, gopts GlobalOptions) {
	t.Logf("backing up %v", target)
	OK(t, runBackup(opts, gopts, target))
}

func testRunList(t testing.TB, tpe string, opts GlobalOptions) restic.IDs {
	buf := bytes.NewBuffer(nil)
	globalOptions.stdout = buf
	defer func() {
		globalOptions.stdout = os.Stdout
	}()

	OK(t, runList(opts, []string{tpe}))
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

	OK(t, runRestore(opts, gopts, []string{"latest"}))
}

func testRunRestoreExcludes(t testing.TB, gopts GlobalOptions, dir string, snapshotID restic.ID, excludes []string) {
	opts := RestoreOptions{
		Target:  dir,
		Exclude: excludes,
	}

	OK(t, runRestore(opts, gopts, []string{snapshotID.String()}))
}

func testRunRestoreIncludes(t testing.TB, gopts GlobalOptions, dir string, snapshotID restic.ID, includes []string) {
	opts := RestoreOptions{
		Target:  dir,
		Include: includes,
	}

	OK(t, runRestore(opts, gopts, []string{snapshotID.String()}))
}

func testRunCheck(t testing.TB, gopts GlobalOptions) {
	opts := CheckOptions{
		ReadData:    true,
		CheckUnused: true,
	}
	OK(t, runCheck(opts, gopts, nil))
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

	OK(t, runRebuildIndex(gopts))
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

	OK(t, runLs(opts, gopts, []string{snapshotID}))

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

	OK(t, runFind(opts, gopts, []string{pattern}))

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

	OK(t, runSnapshots(opts, globalOptions, []string{}))

	snapshots := []Snapshot{}
	OK(t, json.Unmarshal(buf.Bytes(), &snapshots))

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
	OK(t, runForget(opts, gopts, args))
}

func testRunPrune(t testing.TB, gopts GlobalOptions) {
	OK(t, runPrune(gopts))
}

func TestBackup(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(errors.Cause(err)) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		testRunInit(t, gopts)

		SetupTarTestFixture(t, env.testdata, datafile)
		opts := BackupOptions{}

		// first backup
		testRunBackup(t, []string{env.testdata}, opts, gopts)
		snapshotIDs := testRunList(t, "snapshots", gopts)
		Assert(t, len(snapshotIDs) == 1,
			"expected one snapshot, got %v", snapshotIDs)

		testRunCheck(t, gopts)
		stat1 := dirStats(env.repo)

		// second backup, implicit incremental
		testRunBackup(t, []string{env.testdata}, opts, gopts)
		snapshotIDs = testRunList(t, "snapshots", gopts)
		Assert(t, len(snapshotIDs) == 2,
			"expected two snapshots, got %v", snapshotIDs)

		stat2 := dirStats(env.repo)
		if stat2.size > stat1.size+stat1.size/10 {
			t.Error("repository size has grown by more than 10 percent")
		}
		t.Logf("repository grown by %d bytes", stat2.size-stat1.size)

		testRunCheck(t, gopts)
		// third backup, explicit incremental
		opts.Parent = snapshotIDs[0].String()
		testRunBackup(t, []string{env.testdata}, opts, gopts)
		snapshotIDs = testRunList(t, "snapshots", gopts)
		Assert(t, len(snapshotIDs) == 3,
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
			testRunRestore(t, gopts, restoredir, snapshotIDs[0])
			Assert(t, directoriesEqualContents(env.testdata, filepath.Join(restoredir, "testdata")),
				"directories are not equal")
		}

		testRunCheck(t, gopts)
	})
}

func TestBackupNonExistingFile(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(errors.Cause(err)) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		SetupTarTestFixture(t, env.testdata, datafile)

		testRunInit(t, gopts)
		globalOptions.stderr = ioutil.Discard
		defer func() {
			globalOptions.stderr = os.Stderr
		}()

		p := filepath.Join(env.testdata, "0", "0")
		dirs := []string{
			filepath.Join(p, "0"),
			filepath.Join(p, "1"),
			filepath.Join(p, "nonexisting"),
			filepath.Join(p, "5"),
		}

		opts := BackupOptions{}

		testRunBackup(t, dirs, opts, gopts)
	})
}

func TestBackupMissingFile1(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(errors.Cause(err)) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		SetupTarTestFixture(t, env.testdata, datafile)

		testRunInit(t, gopts)
		globalOptions.stderr = ioutil.Discard
		defer func() {
			globalOptions.stderr = os.Stderr
		}()

		ranHook := false
		debug.Hook("pipe.walk1", func(context interface{}) {
			pathname := context.(string)

			if pathname != filepath.Join("testdata", "0", "0", "9") {
				return
			}

			t.Logf("in hook, removing test file testdata/0/0/9/37")
			ranHook = true

			OK(t, os.Remove(filepath.Join(env.testdata, "0", "0", "9", "37")))
		})

		opts := BackupOptions{}

		testRunBackup(t, []string{env.testdata}, opts, gopts)
		testRunCheck(t, gopts)

		Assert(t, ranHook, "hook did not run")
		debug.RemoveHook("pipe.walk1")
	})
}

func TestBackupMissingFile2(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(errors.Cause(err)) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		SetupTarTestFixture(t, env.testdata, datafile)

		testRunInit(t, gopts)

		globalOptions.stderr = ioutil.Discard
		defer func() {
			globalOptions.stderr = os.Stderr
		}()

		ranHook := false
		debug.Hook("pipe.walk2", func(context interface{}) {
			pathname := context.(string)

			if pathname != filepath.Join("testdata", "0", "0", "9", "37") {
				return
			}

			t.Logf("in hook, removing test file testdata/0/0/9/37")
			ranHook = true

			OK(t, os.Remove(filepath.Join(env.testdata, "0", "0", "9", "37")))
		})

		opts := BackupOptions{}

		testRunBackup(t, []string{env.testdata}, opts, gopts)
		testRunCheck(t, gopts)

		Assert(t, ranHook, "hook did not run")
		debug.RemoveHook("pipe.walk2")
	})
}

func TestBackupChangedFile(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(errors.Cause(err)) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		SetupTarTestFixture(t, env.testdata, datafile)

		testRunInit(t, gopts)

		globalOptions.stderr = ioutil.Discard
		defer func() {
			globalOptions.stderr = os.Stderr
		}()

		modFile := filepath.Join(env.testdata, "0", "0", "6", "18")

		ranHook := false
		debug.Hook("archiver.SaveFile", func(context interface{}) {
			pathname := context.(string)

			if pathname != modFile {
				return
			}

			t.Logf("in hook, modifying test file %v", modFile)
			ranHook = true

			OK(t, ioutil.WriteFile(modFile, []byte("modified"), 0600))
		})

		opts := BackupOptions{}

		testRunBackup(t, []string{env.testdata}, opts, gopts)
		testRunCheck(t, gopts)

		Assert(t, ranHook, "hook did not run")
		debug.RemoveHook("archiver.SaveFile")
	})
}

func TestBackupDirectoryError(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(errors.Cause(err)) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		SetupTarTestFixture(t, env.testdata, datafile)

		testRunInit(t, gopts)

		globalOptions.stderr = ioutil.Discard
		defer func() {
			globalOptions.stderr = os.Stderr
		}()

		ranHook := false

		testdir := filepath.Join(env.testdata, "0", "0", "9")

		// install hook that removes the dir right before readdirnames()
		debug.Hook("pipe.readdirnames", func(context interface{}) {
			path := context.(string)

			if path != testdir {
				return
			}

			t.Logf("in hook, removing test file %v", testdir)
			ranHook = true

			OK(t, os.RemoveAll(testdir))
		})

		testRunBackup(t, []string{filepath.Join(env.testdata, "0", "0")}, BackupOptions{}, gopts)
		testRunCheck(t, gopts)

		Assert(t, ranHook, "hook did not run")
		debug.RemoveHook("pipe.walk2")

		snapshots := testRunList(t, "snapshots", gopts)
		Assert(t, len(snapshots) > 0,
			"no snapshots found in repo (%v)", datafile)

		files := testRunLs(t, gopts, snapshots[0].String())

		Assert(t, len(files) > 1, "snapshot is empty")
	})
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
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		testRunInit(t, gopts)

		datadir := filepath.Join(env.base, "testdata")

		for _, filename := range backupExcludeFilenames {
			fp := filepath.Join(datadir, filename)
			OK(t, os.MkdirAll(filepath.Dir(fp), 0755))

			f, err := os.Create(fp)
			OK(t, err)

			fmt.Fprintf(f, filename)
			OK(t, f.Close())
		}

		snapshots := make(map[string]struct{})

		opts := BackupOptions{}

		testRunBackup(t, []string{datadir}, opts, gopts)
		snapshots, snapshotID := lastSnapshot(snapshots, loadSnapshotMap(t, gopts))
		files := testRunLs(t, gopts, snapshotID)
		Assert(t, includes(files, filepath.Join(string(filepath.Separator), "testdata", "foo.tar.gz")),
			"expected file %q in first snapshot, but it's not included", "foo.tar.gz")

		opts.Excludes = []string{"*.tar.gz"}
		testRunBackup(t, []string{datadir}, opts, gopts)
		snapshots, snapshotID = lastSnapshot(snapshots, loadSnapshotMap(t, gopts))
		files = testRunLs(t, gopts, snapshotID)
		Assert(t, !includes(files, filepath.Join(string(filepath.Separator), "testdata", "foo.tar.gz")),
			"expected file %q not in first snapshot, but it's included", "foo.tar.gz")

		opts.Excludes = []string{"*.tar.gz", "private/secret"}
		testRunBackup(t, []string{datadir}, opts, gopts)
		_, snapshotID = lastSnapshot(snapshots, loadSnapshotMap(t, gopts))
		files = testRunLs(t, gopts, snapshotID)
		Assert(t, !includes(files, filepath.Join(string(filepath.Separator), "testdata", "foo.tar.gz")),
			"expected file %q not in first snapshot, but it's included", "foo.tar.gz")
		Assert(t, !includes(files, filepath.Join(string(filepath.Separator), "testdata", "private", "secret", "passwords.txt")),
			"expected file %q not in first snapshot, but it's included", "passwords.txt")
	})
}

const (
	incrementalFirstWrite  = 20 * 1042 * 1024
	incrementalSecondWrite = 12 * 1042 * 1024
	incrementalThirdWrite  = 4 * 1042 * 1024
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
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		testRunInit(t, gopts)

		datadir := filepath.Join(env.base, "testdata")
		testfile := filepath.Join(datadir, "testfile")

		OK(t, appendRandomData(testfile, incrementalFirstWrite))

		opts := BackupOptions{}

		testRunBackup(t, []string{datadir}, opts, gopts)
		testRunCheck(t, gopts)
		stat1 := dirStats(env.repo)

		OK(t, appendRandomData(testfile, incrementalSecondWrite))

		testRunBackup(t, []string{datadir}, opts, gopts)
		testRunCheck(t, gopts)
		stat2 := dirStats(env.repo)
		if stat2.size-stat1.size > incrementalFirstWrite {
			t.Errorf("repository size has grown by more than %d bytes", incrementalFirstWrite)
		}
		t.Logf("repository grown by %d bytes", stat2.size-stat1.size)

		OK(t, appendRandomData(testfile, incrementalThirdWrite))

		testRunBackup(t, []string{datadir}, opts, gopts)
		testRunCheck(t, gopts)
		stat3 := dirStats(env.repo)
		if stat3.size-stat2.size > incrementalFirstWrite {
			t.Errorf("repository size has grown by more than %d bytes", incrementalFirstWrite)
		}
		t.Logf("repository grown by %d bytes", stat3.size-stat2.size)
	})
}

func TestBackupTags(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		testRunInit(t, gopts)
		SetupTarTestFixture(t, env.testdata, datafile)

		opts := BackupOptions{}

		testRunBackup(t, []string{env.testdata}, opts, gopts)
		testRunCheck(t, gopts)
		newest, _ := testRunSnapshots(t, gopts)
		Assert(t, newest != nil, "expected a new backup, got nil")
		Assert(t, len(newest.Tags) == 0,
			"expected no tags, got %v", newest.Tags)

		opts.Tags = []string{"NL"}
		testRunBackup(t, []string{env.testdata}, opts, gopts)
		testRunCheck(t, gopts)
		newest, _ = testRunSnapshots(t, gopts)
		Assert(t, newest != nil, "expected a new backup, got nil")
		Assert(t, len(newest.Tags) == 1 && newest.Tags[0] == "NL",
			"expected one NL tag, got %v", newest.Tags)
	})
}

func testRunTag(t testing.TB, opts TagOptions, gopts GlobalOptions) {
	OK(t, runTag(opts, gopts, []string{}))
}

func TestTag(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		testRunInit(t, gopts)
		SetupTarTestFixture(t, env.testdata, datafile)

		testRunBackup(t, []string{env.testdata}, BackupOptions{}, gopts)
		testRunCheck(t, gopts)
		newest, _ := testRunSnapshots(t, gopts)
		Assert(t, newest != nil, "expected a new backup, got nil")
		Assert(t, len(newest.Tags) == 0,
			"expected no tags, got %v", newest.Tags)
		Assert(t, newest.Original == nil,
			"expected original ID to be nil, got %v", newest.Original)
		originalID := *newest.ID

		testRunTag(t, TagOptions{SetTags: []string{"NL"}}, gopts)
		testRunCheck(t, gopts)
		newest, _ = testRunSnapshots(t, gopts)
		Assert(t, newest != nil, "expected a new backup, got nil")
		Assert(t, len(newest.Tags) == 1 && newest.Tags[0] == "NL",
			"set failed, expected one NL tag, got %v", newest.Tags)
		Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
		Assert(t, *newest.Original == originalID,
			"expected original ID to be set to the first snapshot id")

		testRunTag(t, TagOptions{AddTags: []string{"CH"}}, gopts)
		testRunCheck(t, gopts)
		newest, _ = testRunSnapshots(t, gopts)
		Assert(t, newest != nil, "expected a new backup, got nil")
		Assert(t, len(newest.Tags) == 2 && newest.Tags[0] == "NL" && newest.Tags[1] == "CH",
			"add failed, expected CH,NL tags, got %v", newest.Tags)
		Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
		Assert(t, *newest.Original == originalID,
			"expected original ID to be set to the first snapshot id")

		testRunTag(t, TagOptions{RemoveTags: []string{"NL"}}, gopts)
		testRunCheck(t, gopts)
		newest, _ = testRunSnapshots(t, gopts)
		Assert(t, newest != nil, "expected a new backup, got nil")
		Assert(t, len(newest.Tags) == 1 && newest.Tags[0] == "CH",
			"remove failed, expected one CH tag, got %v", newest.Tags)
		Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
		Assert(t, *newest.Original == originalID,
			"expected original ID to be set to the first snapshot id")

		testRunTag(t, TagOptions{AddTags: []string{"US", "RU"}}, gopts)
		testRunTag(t, TagOptions{RemoveTags: []string{"CH", "US", "RU"}}, gopts)
		testRunCheck(t, gopts)
		newest, _ = testRunSnapshots(t, gopts)
		Assert(t, newest != nil, "expected a new backup, got nil")
		Assert(t, len(newest.Tags) == 0,
			"expected no tags, got %v", newest.Tags)
		Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
		Assert(t, *newest.Original == originalID,
			"expected original ID to be set to the first snapshot id")

		// Check special case of removing all tags.
		testRunTag(t, TagOptions{SetTags: []string{""}}, gopts)
		testRunCheck(t, gopts)
		newest, _ = testRunSnapshots(t, gopts)
		Assert(t, newest != nil, "expected a new backup, got nil")
		Assert(t, len(newest.Tags) == 0,
			"expected no tags, got %v", newest.Tags)
		Assert(t, newest.Original != nil, "expected original snapshot id, got nil")
		Assert(t, *newest.Original == originalID,
			"expected original ID to be set to the first snapshot id")
	})
}

func testRunKeyListOtherIDs(t testing.TB, gopts GlobalOptions) []string {
	buf := bytes.NewBuffer(nil)

	globalOptions.stdout = buf
	defer func() {
		globalOptions.stdout = os.Stdout
	}()

	OK(t, runKey(gopts, []string{"list"}))

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

	OK(t, runKey(gopts, []string{"add"}))
}

func testRunKeyPasswd(t testing.TB, newPassword string, gopts GlobalOptions) {
	testKeyNewPassword = newPassword
	defer func() {
		testKeyNewPassword = ""
	}()

	OK(t, runKey(gopts, []string{"passwd"}))
}

func testRunKeyRemove(t testing.TB, gopts GlobalOptions, IDs []string) {
	t.Logf("remove %d keys: %q\n", len(IDs), IDs)
	for _, id := range IDs {
		OK(t, runKey(gopts, []string{"rm", id}))
	}
}

func TestKeyAddRemove(t *testing.T) {
	passwordList := []string{
		"OnnyiasyatvodsEvVodyawit",
		"raicneirvOjEfEigonOmLasOd",
	}

	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		testRunInit(t, gopts)

		testRunKeyPasswd(t, "geheim2", gopts)
		gopts.password = "geheim2"
		t.Logf("changed password to %q", gopts.password)

		for _, newPassword := range passwordList {
			testRunKeyAddNewKey(t, newPassword, gopts)
			t.Logf("added new password %q", newPassword)
			gopts.password = newPassword
			testRunKeyRemove(t, gopts, testRunKeyListOtherIDs(t, gopts))
		}

		gopts.password = passwordList[len(passwordList)-1]
		t.Logf("testing access with last password %q\n", gopts.password)
		OK(t, runKey(gopts, []string{"list"}))
		testRunCheck(t, gopts)
	})
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

	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		testRunInit(t, gopts)

		for _, test := range testfiles {
			p := filepath.Join(env.testdata, test.name)
			OK(t, os.MkdirAll(filepath.Dir(p), 0755))
			OK(t, appendRandomData(p, test.size))
		}

		opts := BackupOptions{}

		testRunBackup(t, []string{env.testdata}, opts, gopts)
		testRunCheck(t, gopts)

		snapshotID := testRunList(t, "snapshots", gopts)[0]

		// no restore filter should restore all files
		testRunRestore(t, gopts, filepath.Join(env.base, "restore0"), snapshotID)
		for _, test := range testfiles {
			OK(t, testFileSize(filepath.Join(env.base, "restore0", "testdata", test.name), int64(test.size)))
		}

		for i, pat := range []string{"*.c", "*.exe", "*", "*file3*"} {
			base := filepath.Join(env.base, fmt.Sprintf("restore%d", i+1))
			testRunRestoreExcludes(t, gopts, base, snapshotID, []string{pat})
			for _, test := range testfiles {
				err := testFileSize(filepath.Join(base, "testdata", test.name), int64(test.size))
				if ok, _ := filter.Match(pat, filepath.Base(test.name)); !ok {
					OK(t, err)
				} else {
					Assert(t, os.IsNotExist(errors.Cause(err)),
						"expected %v to not exist in restore step %v, but it exists, err %v", test.name, i+1, err)
				}
			}
		}

	})
}

func TestRestore(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		testRunInit(t, gopts)

		for i := 0; i < 10; i++ {
			p := filepath.Join(env.testdata, fmt.Sprintf("foo/bar/testfile%v", i))
			OK(t, os.MkdirAll(filepath.Dir(p), 0755))
			OK(t, appendRandomData(p, uint(mrand.Intn(5<<21))))
		}

		opts := BackupOptions{}

		testRunBackup(t, []string{env.testdata}, opts, gopts)
		testRunCheck(t, gopts)

		// Restore latest without any filters
		restoredir := filepath.Join(env.base, "restore")
		testRunRestoreLatest(t, gopts, restoredir, nil, "")

		Assert(t, directoriesEqualContents(env.testdata, filepath.Join(restoredir, filepath.Base(env.testdata))),
			"directories are not equal")
	})
}

func TestRestoreLatest(t *testing.T) {

	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		testRunInit(t, gopts)

		p := filepath.Join(env.testdata, "testfile.c")
		OK(t, os.MkdirAll(filepath.Dir(p), 0755))
		OK(t, appendRandomData(p, 100))

		opts := BackupOptions{}

		testRunBackup(t, []string{env.testdata}, opts, gopts)
		testRunCheck(t, gopts)

		os.Remove(p)
		OK(t, appendRandomData(p, 101))
		testRunBackup(t, []string{env.testdata}, opts, gopts)
		testRunCheck(t, gopts)

		// Restore latest without any filters
		testRunRestoreLatest(t, gopts, filepath.Join(env.base, "restore0"), nil, "")
		OK(t, testFileSize(filepath.Join(env.base, "restore0", "testdata", "testfile.c"), int64(101)))

		// Setup test files in different directories backed up in different snapshots
		p1 := filepath.Join(env.testdata, "p1/testfile.c")
		OK(t, os.MkdirAll(filepath.Dir(p1), 0755))
		OK(t, appendRandomData(p1, 102))
		testRunBackup(t, []string{filepath.Dir(p1)}, opts, gopts)
		testRunCheck(t, gopts)

		p2 := filepath.Join(env.testdata, "p2/testfile.c")
		OK(t, os.MkdirAll(filepath.Dir(p2), 0755))
		OK(t, appendRandomData(p2, 103))
		testRunBackup(t, []string{filepath.Dir(p2)}, opts, gopts)
		testRunCheck(t, gopts)

		p1rAbs := filepath.Join(env.base, "restore1", "p1/testfile.c")
		p2rAbs := filepath.Join(env.base, "restore2", "p2/testfile.c")

		testRunRestoreLatest(t, gopts, filepath.Join(env.base, "restore1"), []string{filepath.Dir(p1)}, "")
		OK(t, testFileSize(p1rAbs, int64(102)))
		if _, err := os.Stat(p2rAbs); os.IsNotExist(errors.Cause(err)) {
			Assert(t, os.IsNotExist(errors.Cause(err)),
				"expected %v to not exist in restore, but it exists, err %v", p2rAbs, err)
		}

		testRunRestoreLatest(t, gopts, filepath.Join(env.base, "restore2"), []string{filepath.Dir(p2)}, "")
		OK(t, testFileSize(p2rAbs, int64(103)))
		if _, err := os.Stat(p1rAbs); os.IsNotExist(errors.Cause(err)) {
			Assert(t, os.IsNotExist(errors.Cause(err)),
				"expected %v to not exist in restore, but it exists, err %v", p1rAbs, err)
		}

	})
}

func TestRestoreWithPermissionFailure(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		datafile := filepath.Join("testdata", "repo-restore-permissions-test.tar.gz")
		SetupTarTestFixture(t, env.base, datafile)

		snapshots := testRunList(t, "snapshots", gopts)
		Assert(t, len(snapshots) > 0,
			"no snapshots found in repo (%v)", datafile)

		globalOptions.stderr = ioutil.Discard
		defer func() {
			globalOptions.stderr = os.Stderr
		}()

		testRunRestore(t, gopts, filepath.Join(env.base, "restore"), snapshots[0])

		// make sure that all files have been restored, regardless of any
		// permission errors
		files := testRunLs(t, gopts, snapshots[0].String())
		for _, filename := range files {
			fi, err := os.Lstat(filepath.Join(env.base, "restore", filename))
			OK(t, err)

			Assert(t, !isFile(fi) || fi.Size() > 0,
				"file %v restored, but filesize is 0", filename)
		}
	})
}

func setZeroModTime(filename string) error {
	var utimes = []syscall.Timespec{
		syscall.NsecToTimespec(0),
		syscall.NsecToTimespec(0),
	}

	return syscall.UtimesNano(filename, utimes)
}

func TestRestoreNoMetadataOnIgnoredIntermediateDirs(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		testRunInit(t, gopts)

		p := filepath.Join(env.testdata, "subdir1", "subdir2", "subdir3", "file.ext")
		OK(t, os.MkdirAll(filepath.Dir(p), 0755))
		OK(t, appendRandomData(p, 200))
		OK(t, setZeroModTime(filepath.Join(env.testdata, "subdir1", "subdir2")))

		opts := BackupOptions{}

		testRunBackup(t, []string{env.testdata}, opts, gopts)
		testRunCheck(t, gopts)

		snapshotID := testRunList(t, "snapshots", gopts)[0]

		// restore with filter "*.ext", this should restore "file.ext", but
		// since the directories are ignored and only created because of
		// "file.ext", no meta data should be restored for them.
		testRunRestoreIncludes(t, gopts, filepath.Join(env.base, "restore0"), snapshotID, []string{"*.ext"})

		f1 := filepath.Join(env.base, "restore0", "testdata", "subdir1", "subdir2")
		fi, err := os.Stat(f1)
		OK(t, err)

		Assert(t, fi.ModTime() != time.Unix(0, 0),
			"meta data of intermediate directory has been restore although it was ignored")

		// restore with filter "*", this should restore meta data on everything.
		testRunRestoreIncludes(t, gopts, filepath.Join(env.base, "restore1"), snapshotID, []string{"*"})

		f2 := filepath.Join(env.base, "restore1", "testdata", "subdir1", "subdir2")
		fi, err = os.Stat(f2)
		OK(t, err)

		Assert(t, fi.ModTime() == time.Unix(0, 0),
			"meta data of intermediate directory hasn't been restore")
	})
}

func TestFind(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		testRunInit(t, gopts)
		SetupTarTestFixture(t, env.testdata, datafile)

		opts := BackupOptions{}

		testRunBackup(t, []string{env.testdata}, opts, gopts)
		testRunCheck(t, gopts)

		results := testRunFind(t, false, gopts, "unexistingfile")
		Assert(t, len(results) == 0, "unexisting file found in repo (%v)", datafile)

		results = testRunFind(t, false, gopts, "testfile")
		lines := strings.Split(string(results), "\n")
		Assert(t, len(lines) == 2, "expected one file found in repo (%v)", datafile)

		results = testRunFind(t, false, gopts, "testfile*")
		lines = strings.Split(string(results), "\n")
		Assert(t, len(lines) == 4, "expected three files found in repo (%v)", datafile)
	})
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
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		testRunInit(t, gopts)
		SetupTarTestFixture(t, env.testdata, datafile)

		opts := BackupOptions{}

		testRunBackup(t, []string{env.testdata}, opts, gopts)
		testRunCheck(t, gopts)

		results := testRunFind(t, true, gopts, "unexistingfile")
		matches := []testMatches{}
		OK(t, json.Unmarshal(results, &matches))
		Assert(t, len(matches) == 0, "expected no match in repo (%v)", datafile)

		results = testRunFind(t, true, gopts, "testfile")
		OK(t, json.Unmarshal(results, &matches))
		Assert(t, len(matches) == 1, "expected a single snapshot in repo (%v)", datafile)
		Assert(t, len(matches[0].Matches) == 1, "expected a single file to match (%v)", datafile)
		Assert(t, matches[0].Hits == 1, "expected hits to show 1 match (%v)", datafile)

		results = testRunFind(t, true, gopts, "testfile*")
		OK(t, json.Unmarshal(results, &matches))
		Assert(t, len(matches) == 1, "expected a single snapshot in repo (%v)", datafile)
		Assert(t, len(matches[0].Matches) == 3, "expected 3 files to match (%v)", datafile)
		Assert(t, matches[0].Hits == 3, "expected hits to show 3 matches (%v)", datafile)
	})
}

func TestRebuildIndex(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		datafile := filepath.Join("..", "..", "internal", "checker", "testdata", "duplicate-packs-in-index-test-repo.tar.gz")
		SetupTarTestFixture(t, env.base, datafile)

		out, err := testRunCheckOutput(gopts)
		if !strings.Contains(out, "contained in several indexes") {
			t.Fatalf("did not find checker hint for packs in several indexes")
		}

		if err != nil {
			t.Fatalf("expected no error from checker for test repository, got %v", err)
		}

		if !strings.Contains(out, "restic rebuild-index") {
			t.Fatalf("did not find hint for rebuild-index command")
		}

		testRunRebuildIndex(t, gopts)

		out, err = testRunCheckOutput(gopts)
		if len(out) != 0 {
			t.Fatalf("expected no output from the checker, got: %v", out)
		}

		if err != nil {
			t.Fatalf("expected no error from checker after rebuild-index, got: %v", err)
		}
	})
}

func TestRebuildIndexAlwaysFull(t *testing.T) {
	repository.IndexFull = func(*repository.Index) bool { return true }
	TestRebuildIndex(t)
}

func TestCheckRestoreNoLock(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		datafile := filepath.Join("testdata", "small-repo.tar.gz")
		SetupTarTestFixture(t, env.base, datafile)

		err := filepath.Walk(env.repo, func(p string, fi os.FileInfo, e error) error {
			if e != nil {
				return e
			}
			return os.Chmod(p, fi.Mode() & ^(os.FileMode(0222)))
		})
		OK(t, err)

		gopts.NoLock = true

		testRunCheck(t, gopts)

		snapshotIDs := testRunList(t, "snapshots", gopts)
		if len(snapshotIDs) == 0 {
			t.Fatalf("found no snapshots")
		}

		testRunRestore(t, gopts, filepath.Join(env.base, "restore"), snapshotIDs[0])
	})
}

func TestPrune(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(errors.Cause(err)) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		testRunInit(t, gopts)

		SetupTarTestFixture(t, env.testdata, datafile)
		opts := BackupOptions{}

		testRunBackup(t, []string{filepath.Join(env.testdata, "0", "0")}, opts, gopts)
		firstSnapshot := testRunList(t, "snapshots", gopts)
		Assert(t, len(firstSnapshot) == 1,
			"expected one snapshot, got %v", firstSnapshot)

		testRunBackup(t, []string{filepath.Join(env.testdata, "0", "0", "2")}, opts, gopts)
		testRunBackup(t, []string{filepath.Join(env.testdata, "0", "0", "3")}, opts, gopts)

		snapshotIDs := testRunList(t, "snapshots", gopts)
		Assert(t, len(snapshotIDs) == 3,
			"expected 3 snapshot, got %v", snapshotIDs)

		testRunForget(t, gopts, firstSnapshot[0].String())
		testRunPrune(t, gopts)
		testRunCheck(t, gopts)
	})
}

func TestHardLink(t *testing.T) {
	// this test assumes a test set with a single directory containing hard linked files
	withTestEnvironment(t, func(env *testEnvironment, gopts GlobalOptions) {
		datafile := filepath.Join("testdata", "test.hl.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(errors.Cause(err)) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		testRunInit(t, gopts)

		SetupTarTestFixture(t, env.testdata, datafile)

		linkTests := createFileSetPerHardlink(env.testdata)

		opts := BackupOptions{}

		// first backup
		testRunBackup(t, []string{env.testdata}, opts, gopts)
		snapshotIDs := testRunList(t, "snapshots", gopts)
		Assert(t, len(snapshotIDs) == 1,
			"expected one snapshot, got %v", snapshotIDs)

		testRunCheck(t, gopts)

		// restore all backups and compare
		for i, snapshotID := range snapshotIDs {
			restoredir := filepath.Join(env.base, fmt.Sprintf("restore%d", i))
			t.Logf("restoring snapshot %v to %v", snapshotID.Str(), restoredir)
			testRunRestore(t, gopts, restoredir, snapshotIDs[0])
			Assert(t, directoriesEqualContents(env.testdata, filepath.Join(restoredir, "testdata")),
				"directories are not equal")

			linkResults := createFileSetPerHardlink(filepath.Join(restoredir, "testdata"))
			Assert(t, linksEqual(linkTests, linkResults),
				"links are not equal")
		}

		testRunCheck(t, gopts)
	})
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
