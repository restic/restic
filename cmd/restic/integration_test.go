package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/restic/restic/backend"
	. "github.com/restic/restic/test"
)

func setupTarTestFixture(t testing.TB, outputDir, tarFile string) {
	err := system("sh", "-c", `(cd "$1" && tar xz) < "$2"`,
		"sh", outputDir, tarFile)
	OK(t, err)
}

func system(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

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

func cmdBackup(t testing.TB, global GlobalOptions, target []string, parentID backend.ID) {
	cmd := &CmdBackup{global: &global}
	cmd.Parent = parentID.String()

	t.Logf("backing up %v", target)

	OK(t, cmd.Execute(target))
}

func cmdList(t testing.TB, global GlobalOptions, tpe string) []backend.ID {
	var buf bytes.Buffer
	global.stdout = &buf
	cmd := &CmdList{global: &global}

	OK(t, cmd.Execute([]string{tpe}))
	IDs := parseIDsFromReader(t, &buf)

	return IDs
}

func cmdRestore(t testing.TB, global GlobalOptions, dir string, snapshotID backend.ID) {
	cmd := &CmdRestore{global: &global}
	cmd.Execute([]string{snapshotID.String(), dir})
}

func cmdFsck(t testing.TB, global GlobalOptions) {
	cmd := &CmdFsck{global: &global, CheckData: true, Orphaned: true}
	OK(t, cmd.Execute(nil))
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

		setupTarTestFixture(t, env.testdata, datafile)

		// first backup
		cmdBackup(t, global, []string{env.testdata}, nil)
		snapshotIDs := cmdList(t, global, "snapshots")
		Assert(t, len(snapshotIDs) == 1,
			"expected one snapshot, got %v", snapshotIDs)

		cmdFsck(t, global)
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

		cmdFsck(t, global)
		// third backup, explicit incremental
		cmdBackup(t, global, []string{env.testdata}, snapshotIDs[0])
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

		cmdFsck(t, global)
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

		setupTarTestFixture(t, env.testdata, datafile)

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
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(err) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		cmdInit(t, global)

		datadir := filepath.Join(env.base, "testdata")
		testfile := filepath.Join(datadir, "testfile")

		OK(t, appendRandomData(testfile, incrementalFirstWrite))

		cmdBackup(t, global, []string{datadir}, nil)
		cmdFsck(t, global)
		stat1 := dirStats(env.repo)

		OK(t, appendRandomData(testfile, incrementalSecondWrite))

		cmdBackup(t, global, []string{datadir}, nil)
		cmdFsck(t, global)
		stat2 := dirStats(env.repo)
		if stat2.size-stat1.size > incrementalFirstWrite {
			t.Errorf("repository size has grown by more than %d bytes", incrementalFirstWrite)
		}
		t.Logf("repository grown by %d bytes", stat2.size-stat1.size)

		OK(t, appendRandomData(testfile, incrementalThirdWrite))

		cmdBackup(t, global, []string{datadir}, nil)
		cmdFsck(t, global)
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

		cmdFsck(t, global)
	})
}
