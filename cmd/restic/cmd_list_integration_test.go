package main

import (
	"bufio"
	"context"
	"io"
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

func testListSnapshots(t testing.TB, opts GlobalOptions, expected int) restic.IDs {
	t.Helper()
	snapshotIDs := testRunList(t, "snapshots", opts)
	rtest.Assert(t, len(snapshotIDs) == expected, "expected %v snapshot, got %v", expected, snapshotIDs)
	return snapshotIDs
}
