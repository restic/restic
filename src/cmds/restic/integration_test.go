package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"restic/backend"
	"restic/debug"
	"restic/filter"
	"restic/repository"
	. "restic/test"
)

func parseIDsFromReader(t testing.TB, rd io.Reader) backend.IDs {
	IDs := backend.IDs{}
	sc := bufio.NewScanner(rd)

	for sc.Scan() {
		id, err := backend.ParseID(sc.Text())
		if err != nil {
			t.Logf("parse id %v: %v", sc.Text(), err)
			continue
		}

		IDs = append(IDs, id)
	}

	return IDs
}

func cmdInit(t testing.TB, global GlobalOptions) {
	cmd := &CmdInit{global: &global}
	OK(t, cmd.Execute(nil))

	t.Logf("repository initialized at %v", global.Repo)
}

func cmdBackup(t testing.TB, global GlobalOptions, target []string, parentID *backend.ID) {
	cmdBackupExcludes(t, global, target, parentID, nil)
}

func cmdBackupExcludes(t testing.TB, global GlobalOptions, target []string, parentID *backend.ID, excludes []string) {
	cmd := &CmdBackup{global: &global, Excludes: excludes}
	if parentID != nil {
		cmd.Parent = parentID.String()
	}

	t.Logf("backing up %v", target)

	OK(t, cmd.Execute(target))
}

func cmdList(t testing.TB, global GlobalOptions, tpe string) backend.IDs {
	cmd := &CmdList{global: &global}
	return executeAndParseIDs(t, cmd, tpe)
}

func executeAndParseIDs(t testing.TB, cmd *CmdList, args ...string) backend.IDs {
	buf := bytes.NewBuffer(nil)
	cmd.global.stdout = buf
	OK(t, cmd.Execute(args))
	return parseIDsFromReader(t, buf)
}

func cmdRestore(t testing.TB, global GlobalOptions, dir string, snapshotID backend.ID) {
	cmdRestoreExcludes(t, global, dir, snapshotID, nil)
}

func cmdRestoreExcludes(t testing.TB, global GlobalOptions, dir string, snapshotID backend.ID, excludes []string) {
	cmd := &CmdRestore{global: &global, Target: dir, Exclude: excludes}
	OK(t, cmd.Execute([]string{snapshotID.String()}))
}

func cmdRestoreIncludes(t testing.TB, global GlobalOptions, dir string, snapshotID backend.ID, includes []string) {
	cmd := &CmdRestore{global: &global, Target: dir, Include: includes}
	OK(t, cmd.Execute([]string{snapshotID.String()}))
}

func cmdCheck(t testing.TB, global GlobalOptions) {
	cmd := &CmdCheck{
		global:      &global,
		ReadData:    true,
		CheckUnused: true,
	}
	OK(t, cmd.Execute(nil))
}

func cmdCheckOutput(t testing.TB, global GlobalOptions) string {
	buf := bytes.NewBuffer(nil)
	global.stdout = buf
	cmd := &CmdCheck{global: &global, ReadData: true}
	OK(t, cmd.Execute(nil))
	return string(buf.Bytes())
}

func cmdRebuildIndex(t testing.TB, global GlobalOptions) {
	global.stdout = ioutil.Discard
	cmd := &CmdRebuildIndex{global: &global}
	OK(t, cmd.Execute(nil))
}

func cmdOptimize(t testing.TB, global GlobalOptions) {
	cmd := &CmdOptimize{global: &global}
	OK(t, cmd.Execute(nil))
}

func cmdLs(t testing.TB, global GlobalOptions, snapshotID string) []string {
	var buf bytes.Buffer
	global.stdout = &buf

	cmd := &CmdLs{global: &global}
	OK(t, cmd.Execute([]string{snapshotID}))

	return strings.Split(string(buf.Bytes()), "\n")
}

func cmdFind(t testing.TB, global GlobalOptions, pattern string) []string {
	var buf bytes.Buffer
	global.stdout = &buf

	cmd := &CmdFind{global: &global}
	OK(t, cmd.Execute([]string{pattern}))

	return strings.Split(string(buf.Bytes()), "\n")
}

func TestBackup(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(err) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		cmdInit(t, global)

		SetupTarTestFixture(t, env.testdata, datafile)

		// first backup
		cmdBackup(t, global, []string{env.testdata}, nil)
		snapshotIDs := cmdList(t, global, "snapshots")
		Assert(t, len(snapshotIDs) == 1,
			"expected one snapshot, got %v", snapshotIDs)

		cmdCheck(t, global)
		stat1 := dirStats(env.repo)

		// second backup, implicit incremental
		cmdBackup(t, global, []string{env.testdata}, nil)
		snapshotIDs = cmdList(t, global, "snapshots")
		Assert(t, len(snapshotIDs) == 2,
			"expected two snapshots, got %v", snapshotIDs)

		stat2 := dirStats(env.repo)
		if stat2.size > stat1.size+stat1.size/10 {
			t.Error("repository size has grown by more than 10 percent")
		}
		t.Logf("repository grown by %d bytes", stat2.size-stat1.size)

		cmdCheck(t, global)
		// third backup, explicit incremental
		cmdBackup(t, global, []string{env.testdata}, &snapshotIDs[0])
		snapshotIDs = cmdList(t, global, "snapshots")
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
			cmdRestore(t, global, restoredir, snapshotIDs[0])
			Assert(t, directoriesEqualContents(env.testdata, filepath.Join(restoredir, "testdata")),
				"directories are not equal")
		}

		cmdCheck(t, global)
	})
}

func TestBackupNonExistingFile(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(err) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		SetupTarTestFixture(t, env.testdata, datafile)

		cmdInit(t, global)

		global.stderr = ioutil.Discard

		p := filepath.Join(env.testdata, "0", "0")
		dirs := []string{
			filepath.Join(p, "0"),
			filepath.Join(p, "1"),
			filepath.Join(p, "nonexisting"),
			filepath.Join(p, "5"),
		}
		cmdBackup(t, global, dirs, nil)
	})
}

func TestBackupMissingFile1(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(err) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		SetupTarTestFixture(t, env.testdata, datafile)

		cmdInit(t, global)

		global.stderr = ioutil.Discard
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

		cmdBackup(t, global, []string{env.testdata}, nil)
		cmdCheck(t, global)

		Assert(t, ranHook, "hook did not run")
		debug.RemoveHook("pipe.walk1")
	})
}

func TestBackupMissingFile2(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(err) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		SetupTarTestFixture(t, env.testdata, datafile)

		cmdInit(t, global)

		global.stderr = ioutil.Discard
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

		cmdBackup(t, global, []string{env.testdata}, nil)
		cmdCheck(t, global)

		Assert(t, ranHook, "hook did not run")
		debug.RemoveHook("pipe.walk2")
	})
}

func TestBackupDirectoryError(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(err) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		SetupTarTestFixture(t, env.testdata, datafile)

		cmdInit(t, global)

		global.stderr = ioutil.Discard
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

		cmdBackup(t, global, []string{filepath.Join(env.testdata, "0", "0")}, nil)
		cmdCheck(t, global)

		Assert(t, ranHook, "hook did not run")
		debug.RemoveHook("pipe.walk2")

		snapshots := cmdList(t, global, "snapshots")
		Assert(t, len(snapshots) > 0,
			"no snapshots found in repo (%v)", datafile)

		files := cmdLs(t, global, snapshots[0].String())

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

func loadSnapshotMap(t testing.TB, global GlobalOptions) map[string]struct{} {
	snapshotIDs := cmdList(t, global, "snapshots")

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
	withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
		cmdInit(t, global)

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

		cmdBackup(t, global, []string{datadir}, nil)
		snapshots, snapshotID := lastSnapshot(snapshots, loadSnapshotMap(t, global))
		files := cmdLs(t, global, snapshotID)
		Assert(t, includes(files, filepath.Join("testdata", "foo.tar.gz")),
			"expected file %q in first snapshot, but it's not included", "foo.tar.gz")

		cmdBackupExcludes(t, global, []string{datadir}, nil, []string{"*.tar.gz"})
		snapshots, snapshotID = lastSnapshot(snapshots, loadSnapshotMap(t, global))
		files = cmdLs(t, global, snapshotID)
		Assert(t, !includes(files, filepath.Join("testdata", "foo.tar.gz")),
			"expected file %q not in first snapshot, but it's included", "foo.tar.gz")

		cmdBackupExcludes(t, global, []string{datadir}, nil, []string{"*.tar.gz", "private/secret"})
		snapshots, snapshotID = lastSnapshot(snapshots, loadSnapshotMap(t, global))
		files = cmdLs(t, global, snapshotID)
		Assert(t, !includes(files, filepath.Join("testdata", "foo.tar.gz")),
			"expected file %q not in first snapshot, but it's included", "foo.tar.gz")
		Assert(t, !includes(files, filepath.Join("testdata", "private", "secret", "passwords.txt")),
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
	withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
		cmdInit(t, global)

		datadir := filepath.Join(env.base, "testdata")
		testfile := filepath.Join(datadir, "testfile")

		OK(t, appendRandomData(testfile, incrementalFirstWrite))

		cmdBackup(t, global, []string{datadir}, nil)
		cmdCheck(t, global)
		stat1 := dirStats(env.repo)

		OK(t, appendRandomData(testfile, incrementalSecondWrite))

		cmdBackup(t, global, []string{datadir}, nil)
		cmdCheck(t, global)
		stat2 := dirStats(env.repo)
		if stat2.size-stat1.size > incrementalFirstWrite {
			t.Errorf("repository size has grown by more than %d bytes", incrementalFirstWrite)
		}
		t.Logf("repository grown by %d bytes", stat2.size-stat1.size)

		OK(t, appendRandomData(testfile, incrementalThirdWrite))

		cmdBackup(t, global, []string{datadir}, nil)
		cmdCheck(t, global)
		stat3 := dirStats(env.repo)
		if stat3.size-stat2.size > incrementalFirstWrite {
			t.Errorf("repository size has grown by more than %d bytes", incrementalFirstWrite)
		}
		t.Logf("repository grown by %d bytes", stat3.size-stat2.size)
	})
}

func cmdKey(t testing.TB, global GlobalOptions, args ...string) string {
	var buf bytes.Buffer

	global.stdout = &buf
	cmd := &CmdKey{global: &global}
	OK(t, cmd.Execute(args))

	return buf.String()
}

func cmdKeyListOtherIDs(t testing.TB, global GlobalOptions) []string {
	var buf bytes.Buffer

	global.stdout = &buf
	cmd := &CmdKey{global: &global}
	OK(t, cmd.Execute([]string{"list"}))

	scanner := bufio.NewScanner(&buf)
	exp := regexp.MustCompile(`^ ([a-f0-9]+) `)

	IDs := []string{}
	for scanner.Scan() {
		if id := exp.FindStringSubmatch(scanner.Text()); id != nil {
			IDs = append(IDs, id[1])
		}
	}

	return IDs
}

func cmdKeyAddNewKey(t testing.TB, global GlobalOptions, newPassword string) {
	cmd := &CmdKey{global: &global, newPassword: newPassword}
	OK(t, cmd.Execute([]string{"add"}))
}

func cmdKeyPasswd(t testing.TB, global GlobalOptions, newPassword string) {
	cmd := &CmdKey{global: &global, newPassword: newPassword}
	OK(t, cmd.Execute([]string{"passwd"}))
}

func cmdKeyRemove(t testing.TB, global GlobalOptions, IDs []string) {
	cmd := &CmdKey{global: &global}
	t.Logf("remove %d keys: %q\n", len(IDs), IDs)
	for _, id := range IDs {
		OK(t, cmd.Execute([]string{"rm", id}))
	}
}

func TestKeyAddRemove(t *testing.T) {
	passwordList := []string{
		"OnnyiasyatvodsEvVodyawit",
		"raicneirvOjEfEigonOmLasOd",
	}

	withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
		cmdInit(t, global)

		cmdKeyPasswd(t, global, "geheim2")
		global.password = "geheim2"
		t.Logf("changed password to %q", global.password)

		for _, newPassword := range passwordList {
			cmdKeyAddNewKey(t, global, newPassword)
			t.Logf("added new password %q", newPassword)
			global.password = newPassword
			cmdKeyRemove(t, global, cmdKeyListOtherIDs(t, global))
		}

		global.password = passwordList[len(passwordList)-1]
		t.Logf("testing access with last password %q\n", global.password)
		cmdKey(t, global, "list")

		cmdCheck(t, global)
	})
}

func testFileSize(filename string, size int64) error {
	fi, err := os.Stat(filename)
	if err != nil {
		return err
	}

	if fi.Size() != size {
		return fmt.Errorf("wrong file size for %v: expected %v, got %v", filename, size, fi.Size())
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

	withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
		cmdInit(t, global)

		for _, test := range testfiles {
			p := filepath.Join(env.testdata, test.name)
			OK(t, os.MkdirAll(filepath.Dir(p), 0755))
			OK(t, appendRandomData(p, test.size))
		}

		cmdBackup(t, global, []string{env.testdata}, nil)
		cmdCheck(t, global)

		snapshotID := cmdList(t, global, "snapshots")[0]

		// no restore filter should restore all files
		cmdRestore(t, global, filepath.Join(env.base, "restore0"), snapshotID)
		for _, test := range testfiles {
			OK(t, testFileSize(filepath.Join(env.base, "restore0", "testdata", test.name), int64(test.size)))
		}

		for i, pat := range []string{"*.c", "*.exe", "*", "*file3*"} {
			base := filepath.Join(env.base, fmt.Sprintf("restore%d", i+1))
			cmdRestoreExcludes(t, global, base, snapshotID, []string{pat})
			for _, test := range testfiles {
				err := testFileSize(filepath.Join(base, "testdata", test.name), int64(test.size))
				if ok, _ := filter.Match(pat, filepath.Base(test.name)); !ok {
					OK(t, err)
				} else {
					Assert(t, os.IsNotExist(err),
						"expected %v to not exist in restore step %v, but it exists, err %v", test.name, i+1, err)
				}
			}
		}

	})
}

func TestRestoreWithPermissionFailure(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
		datafile := filepath.Join("testdata", "repo-restore-permissions-test.tar.gz")
		SetupTarTestFixture(t, env.base, datafile)

		snapshots := cmdList(t, global, "snapshots")
		Assert(t, len(snapshots) > 0,
			"no snapshots found in repo (%v)", datafile)

		global.stderr = ioutil.Discard
		cmdRestore(t, global, filepath.Join(env.base, "restore"), snapshots[0])

		// make sure that all files have been restored, regardeless of any
		// permission errors
		files := cmdLs(t, global, snapshots[0].String())
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
	withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
		cmdInit(t, global)

		p := filepath.Join(env.testdata, "subdir1", "subdir2", "subdir3", "file.ext")
		OK(t, os.MkdirAll(filepath.Dir(p), 0755))
		OK(t, appendRandomData(p, 200))
		OK(t, setZeroModTime(filepath.Join(env.testdata, "subdir1", "subdir2")))

		cmdBackup(t, global, []string{env.testdata}, nil)
		cmdCheck(t, global)

		snapshotID := cmdList(t, global, "snapshots")[0]

		// restore with filter "*.ext", this should restore "file.ext", but
		// since the directories are ignored and only created because of
		// "file.ext", no meta data should be restored for them.
		cmdRestoreIncludes(t, global, filepath.Join(env.base, "restore0"), snapshotID, []string{"*.ext"})

		f1 := filepath.Join(env.base, "restore0", "testdata", "subdir1", "subdir2")
		fi, err := os.Stat(f1)
		OK(t, err)

		Assert(t, fi.ModTime() != time.Unix(0, 0),
			"meta data of intermediate directory has been restore although it was ignored")

		// restore with filter "*", this should restore meta data on everything.
		cmdRestoreIncludes(t, global, filepath.Join(env.base, "restore1"), snapshotID, []string{"*"})

		f2 := filepath.Join(env.base, "restore1", "testdata", "subdir1", "subdir2")
		fi, err = os.Stat(f2)
		OK(t, err)

		Assert(t, fi.ModTime() == time.Unix(0, 0),
			"meta data of intermediate directory hasn't been restore")
	})
}

func TestFind(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		cmdInit(t, global)
		SetupTarTestFixture(t, env.testdata, datafile)
		cmdBackup(t, global, []string{env.testdata}, nil)
		cmdCheck(t, global)

		results := cmdFind(t, global, "unexistingfile")
		Assert(t, len(results) != 0, "unexisting file found in repo (%v)", datafile)

		results = cmdFind(t, global, "testfile")
		Assert(t, len(results) != 1, "file not found in repo (%v)", datafile)

		results = cmdFind(t, global, "test")
		Assert(t, len(results) < 2, "less than two file found in repo (%v)", datafile)
	})
}

func TestRebuildIndex(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
		datafile := filepath.Join("..", "..", "checker", "testdata", "duplicate-packs-in-index-test-repo.tar.gz")
		SetupTarTestFixture(t, env.base, datafile)

		out := cmdCheckOutput(t, global)
		if !strings.Contains(out, "contained in several indexes") {
			t.Fatalf("did not find checker hint for packs in several indexes")
		}

		if !strings.Contains(out, "restic rebuild-index") {
			t.Fatalf("did not find hint for rebuild-index comman")
		}

		cmdRebuildIndex(t, global)

		out = cmdCheckOutput(t, global)
		if len(out) != 0 {
			t.Fatalf("expected no output from the checker, got: %v", out)
		}
	})
}

func TestRebuildIndexAlwaysFull(t *testing.T) {
	repository.IndexFull = func(*repository.Index) bool { return true }
	TestRebuildIndex(t)
}

var optimizeTests = []struct {
	testFilename string
	snapshots    backend.IDSet
}{
	{
		filepath.Join("..", "..", "checker", "testdata", "checker-test-repo.tar.gz"),
		backend.NewIDSet(ParseID("a13c11e582b77a693dd75ab4e3a3ba96538a056594a4b9076e4cacebe6e06d43")),
	},
	{
		filepath.Join("testdata", "old-index-repo.tar.gz"),
		nil,
	},
	{
		filepath.Join("testdata", "old-index-repo.tar.gz"),
		backend.NewIDSet(
			ParseID("f7d83db709977178c9d1a09e4009355e534cde1a135b8186b8b118a3fc4fcd41"),
			ParseID("51d249d28815200d59e4be7b3f21a157b864dc343353df9d8e498220c2499b02"),
		),
	},
}

func TestOptimizeRemoveUnusedBlobs(t *testing.T) {
	for i, test := range optimizeTests {
		withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
			SetupTarTestFixture(t, env.base, test.testFilename)

			for id := range test.snapshots {
				OK(t, removeFile(filepath.Join(env.repo, "snapshots", id.String())))
			}

			cmdOptimize(t, global)
			output := cmdCheckOutput(t, global)

			if len(output) > 0 {
				t.Errorf("expected no output for check in test %d, got:\n%v", i, output)
			}
		})
	}
}

func TestCheckRestoreNoLock(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment, global GlobalOptions) {
		datafile := filepath.Join("testdata", "small-repo.tar.gz")
		SetupTarTestFixture(t, env.base, datafile)

		err := filepath.Walk(env.repo, func(p string, fi os.FileInfo, e error) error {
			if e != nil {
				return e
			}
			return os.Chmod(p, fi.Mode() & ^(os.FileMode(0222)))
		})
		OK(t, err)

		global.NoLock = true
		cmdCheck(t, global)

		snapshotIDs := cmdList(t, global, "snapshots")
		if len(snapshotIDs) == 0 {
			t.Fatalf("found no snapshots")
		}

		cmdRestore(t, global, filepath.Join(env.base, "restore"), snapshotIDs[0])
	})
}
