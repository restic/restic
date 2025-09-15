package main

import (
	"bufio"
	"context"
	"io"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui"
)

func testRunList(t testing.TB, opts GlobalOptions, tpe string) restic.IDs {
	buf, err := withCaptureStdout(opts, func(opts GlobalOptions) error {
		return withTermStatus(opts, func(ctx context.Context, term ui.Terminal) error {
			return runList(ctx, opts, []string{tpe}, term)
		})
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
	snapshotIDs := testRunList(t, opts, "snapshots")
	rtest.Assert(t, len(snapshotIDs) == expected, "expected %v snapshot, got %v", expected, snapshotIDs)
	return snapshotIDs
}
