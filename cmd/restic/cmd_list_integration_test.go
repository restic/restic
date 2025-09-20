package main

import (
	"bufio"
	"context"
	"io"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunList(t testing.TB, gopts GlobalOptions, tpe string) restic.IDs {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts GlobalOptions) error {
		return runList(ctx, gopts, []string{tpe}, gopts.term)
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

func testListSnapshots(t testing.TB, gopts GlobalOptions, expected int) restic.IDs {
	t.Helper()
	snapshotIDs := testRunList(t, gopts, "snapshots")
	rtest.Assert(t, len(snapshotIDs) == expected, "expected %v snapshot, got %v", expected, snapshotIDs)
	return snapshotIDs
}
