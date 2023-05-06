package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testRunSnapshots(t testing.TB, gopts GlobalOptions) (newest *Snapshot, snapmap map[restic.ID]Snapshot) {
	buf := bytes.NewBuffer(nil)
	globalOptions.stdout = buf
	globalOptions.JSON = true
	defer func() {
		globalOptions.stdout = os.Stdout
		globalOptions.JSON = gopts.JSON
	}()

	opts := SnapshotOptions{}

	rtest.OK(t, runSnapshots(context.TODO(), opts, globalOptions, []string{}))

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
