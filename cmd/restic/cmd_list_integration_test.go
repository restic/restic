package main

import (
	"bufio"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunList(t testing.TB, tpe string, opts GlobalOptions) restic.IDs {
	buf, err := withCaptureStdout(func() error {
		return runList(context.TODO(), opts, []string{tpe})
	})
	rtest.OK(t, err)
	return parseIDsFromReader(t, buf)
}

func parseIDsFromReader(t testing.TB, rd io.Reader) restic.IDs {
	t.Helper()
	IDs := restic.IDs{}
	sc := bufio.NewScanner(rd)

	var id restic.ID
	var err error
	for sc.Scan() {
		if len(sc.Text()) == 64 {
			id, err = restic.ParseID(sc.Text())
			if err != nil {
				t.Logf("parse id %v: %v", sc.Text(), err)
				continue
			}
		} else {
			parts := strings.Split(sc.Text(), " ")
			id, err = restic.ParseID(parts[1])
			if err != nil {
				t.Logf("parse id %v: %v", sc.Text(), err)
				continue
			}
		}

		IDs = append(IDs, id)
	}

	return IDs
}

func testListSnapshots(t testing.TB, opts GlobalOptions, expected int) restic.IDs {
	t.Helper()
	snapshotIDs := testRunList(t, "snapshots", opts)
	rtest.Assert(t, len(snapshotIDs) == expected, "expected %v snapshot, got %v", expected, snapshotIDs)
	return snapshotIDs
}

func TestListBlobs(t *testing.T) {

	env, cleanup := withTestEnvironment(t)
	defer cleanup()

	datafile := testSetupBackupData(t, env)
	_ = datafile
	opts := BackupOptions{}

	// first backup
	testRunBackup(t, "", []string{filepath.Join(env.testdata, "0", "0", "7")}, opts, env.gopts)
	testListSnapshots(t, env.gopts, 1)

	resticIDs := testRunList(t, "blobs", env.gopts)
	testIDSet := restic.NewIDSet(resticIDs...)

	// get repo
	_, repo, unlock, err := openWithReadLock(context.TODO(), env.gopts, false)
	rtest.OK(t, err)
	defer unlock()

	setFromIndex := restic.IDSet{}
	rtest.OK(t, repo.LoadIndex(context.TODO(), nil))
	rtest.OK(t, repo.ListBlobs(context.TODO(), func(blob restic.PackedBlob) {
		setFromIndex.Insert(blob.ID)
	}))

	rtest.Assert(t, setFromIndex.Equals(testIDSet), "the set of restic.ID s should be equal")
}
