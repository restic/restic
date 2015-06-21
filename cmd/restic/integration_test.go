package main

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	rd, wr := io.Pipe()
	global.stdout = wr
	cmd := &CmdList{global: &global}

	go func() {
		OK(t, cmd.Execute([]string{tpe}))
		OK(t, wr.Close())
	}()

	IDs := parseIDsFromReader(t, rd)

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

func cmdKey(t testing.TB, global GlobalOptions, args ...string) {
	cmd := &CmdKey{global: &global}
	OK(t, cmd.Execute(args))
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

func TestInit(t *testing.T) {

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

func TestKeyAddRemove(t *testing.T) {
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
		cmdKey(t, global, "list")
	})
}
