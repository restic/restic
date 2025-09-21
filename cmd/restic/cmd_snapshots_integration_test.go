package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunSnapshots(t testing.TB, gopts GlobalOptions) (newest *Snapshot, snapmap map[restic.ID]Snapshot) {
	buf, err := withCaptureStdout(t, gopts, func(ctx context.Context, gopts GlobalOptions) error {
		gopts.JSON = true

		opts := SnapshotOptions{}
		return runSnapshots(ctx, opts, gopts, []string{}, gopts.term)
	})
	rtest.OK(t, err)

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
