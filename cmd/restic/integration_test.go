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
	err := system("sh", "-c", `mkdir "$1" && (cd "$1" && tar xz) < "$2"`,
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

func cmdInit(t testing.TB) {
	cmd := &CmdInit{}
	OK(t, cmd.Execute(nil))

	t.Logf("repository initialized at %v", opts.Repo)
}

func cmdBackup(t testing.TB, target []string, parentID backend.ID) {
	cmd := &CmdBackup{}
	cmd.Parent = parentID.String()

	t.Logf("backing up %v", target)

	OK(t, cmd.Execute(target))
}

func cmdList(t testing.TB, tpe string) []backend.ID {
	rd, wr := io.Pipe()

	cmd := &CmdList{w: wr}

	go func() {
		OK(t, cmd.Execute([]string{tpe}))
		OK(t, wr.Close())
	}()

	IDs := parseIDsFromReader(t, rd)

	return IDs
}

func cmdRestore(t testing.TB, dir string, snapshotID backend.ID) {
	cmd := &CmdRestore{}
	cmd.Execute([]string{snapshotID.String(), dir})
}

func cmdFsck(t testing.TB) {
	cmd := &CmdFsck{CheckData: true, Orphaned: true}
	OK(t, cmd.Execute(nil))
}

func cmdKey(t testing.TB, args ...string) {
	cmd := &CmdKey{}
	OK(t, cmd.Execute(args))
}

func TestBackup(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(err) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		cmdInit(t)

		datadir := filepath.Join(env.base, "testdata")
		setupTarTestFixture(t, datadir, datafile)

		// first backup
		cmdBackup(t, []string{datadir}, nil)
		snapshotIDs := cmdList(t, "snapshots")
		Assert(t, len(snapshotIDs) == 1,
			"more than one snapshot ID in repo")

		cmdFsck(t)
		stat1 := dirStats(env.repo)

		// second backup, implicit incremental
		cmdBackup(t, []string{datadir}, nil)
		snapshotIDs = cmdList(t, "snapshots")
		Assert(t, len(snapshotIDs) == 2,
			"more than one snapshot ID in repo")

		stat2 := dirStats(env.repo)
		if stat2.size > stat1.size+stat1.size/10 {
			t.Error("repository size has grown by more than 10 percent")
		}
		t.Logf("repository grown by %d bytes", stat2.size-stat1.size)

		cmdFsck(t)
		// third backup, explicit incremental
		cmdBackup(t, []string{datadir}, snapshotIDs[0])
		snapshotIDs = cmdList(t, "snapshots")
		Assert(t, len(snapshotIDs) == 3,
			"more than two snapshot IDs in repo")

		stat3 := dirStats(env.repo)
		if stat3.size > stat1.size+stat1.size/10 {
			t.Error("repository size has grown by more than 10 percent")
		}
		t.Logf("repository grown by %d bytes", stat3.size-stat2.size)

		// restore all backups and compare
		for i, snapshotID := range snapshotIDs {
			restoredir := filepath.Join(env.base, fmt.Sprintf("restore%d", i))
			t.Logf("restoring snapshot %v to %v", snapshotID.Str(), restoredir)
			cmdRestore(t, restoredir, snapshotIDs[0])
			Assert(t, directoriesEqualContents(datadir, filepath.Join(restoredir, "testdata")),
				"directories are not equal")
		}

		cmdFsck(t)
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
	withTestEnvironment(t, func(env *testEnvironment) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(err) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		cmdInit(t)

		datadir := filepath.Join(env.base, "testdata")
		testfile := filepath.Join(datadir, "testfile")

		OK(t, appendRandomData(testfile, incrementalFirstWrite))

		cmdBackup(t, []string{datadir}, nil)
		cmdFsck(t)
		stat1 := dirStats(env.repo)

		OK(t, appendRandomData(testfile, incrementalSecondWrite))

		cmdBackup(t, []string{datadir}, nil)
		cmdFsck(t)
		stat2 := dirStats(env.repo)
		if stat2.size-stat1.size > incrementalFirstWrite {
			t.Errorf("repository size has grown by more than %d bytes", incrementalFirstWrite)
		}
		t.Logf("repository grown by %d bytes", stat2.size-stat1.size)

		OK(t, appendRandomData(testfile, incrementalThirdWrite))

		cmdBackup(t, []string{datadir}, nil)
		cmdFsck(t)
		stat3 := dirStats(env.repo)
		if stat3.size-stat2.size > incrementalFirstWrite {
			t.Errorf("repository size has grown by more than %d bytes", incrementalFirstWrite)
		}
		t.Logf("repository grown by %d bytes", stat3.size-stat2.size)
	})
}

func TestKeyAddRemove(t *testing.T) {
	withTestEnvironment(t, func(env *testEnvironment) {
		datafile := filepath.Join("testdata", "backup-data.tar.gz")
		fd, err := os.Open(datafile)
		if os.IsNotExist(err) {
			t.Skipf("unable to find data file %q, skipping", datafile)
			return
		}
		OK(t, err)
		OK(t, fd.Close())

		cmdInit(t)
		cmdKey(t, "list")
	})
}
