package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/data"
	rtest "github.com/restic/restic/internal/test"
	"github.com/spf13/pflag"
)

// Regression test for #2979: no snapshots should print as [], not null.
func TestSnapshotsEmptySnapshotGroupJSON(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		var w strings.Builder
		err := printSnapshotGroupJSON(&w, nil, grouped)
		rtest.OK(t, err)

		rtest.Equals(t, "[]", strings.TrimSpace(w.String()))
	}
}

// TestSnapshotsPrintSnapshotGroupJSONPreservesOrder verifies that printSnapshotGroupJSON
// does not re-sort the snapshots it is given, since sorting happens centrally in runSnapshots.
func TestSnapshotsPrintSnapshotGroupJSONPreservesOrder(t *testing.T) {
	now := time.Now()
	snapshotGroups := map[string]data.Snapshots{
		"{}": {
			&data.Snapshot{Time: now.Add(2 * time.Hour), Hostname: "host1", Paths: []string{"/data"}},
			&data.Snapshot{Time: now, Hostname: "host1", Paths: []string{"/data"}},
			&data.Snapshot{Time: now.Add(1 * time.Hour), Hostname: "host1", Paths: []string{"/data"}},
		},
	}

	var buf bytes.Buffer
	err := printSnapshotGroupJSON(&buf, snapshotGroups, false)
	rtest.OK(t, err)

	var snapshots []Snapshot
	rtest.OK(t, json.Unmarshal(buf.Bytes(), &snapshots))

	rtest.Assert(t, len(snapshots) == 3, "expected 3 snapshots, got %d", len(snapshots))
	rtest.Assert(t, snapshots[0].Time.Equal(now.Add(2*time.Hour)), "order should be preserved as given")
	rtest.Assert(t, snapshots[1].Time.Equal(now), "order should be preserved as given")
	rtest.Assert(t, snapshots[2].Time.Equal(now.Add(1*time.Hour)), "order should be preserved as given")
}

// TestSnapshotsPrintSnapshotGroupJSONGroupedPreservesOrder verifies that grouped JSON output
// also preserves the order of the snapshots it is given.
func TestSnapshotsPrintSnapshotGroupJSONGroupedPreservesOrder(t *testing.T) {
	now := time.Now()
	snapshotGroups := map[string]data.Snapshots{
		`{"hostname":"host1","tags":null,"paths":null}`: {
			&data.Snapshot{Time: now.Add(2 * time.Hour), Hostname: "host1", Paths: []string{"/data"}},
			&data.Snapshot{Time: now, Hostname: "host1", Paths: []string{"/data"}},
			&data.Snapshot{Time: now.Add(1 * time.Hour), Hostname: "host1", Paths: []string{"/data"}},
		},
	}

	var buf bytes.Buffer
	err := printSnapshotGroupJSON(&buf, snapshotGroups, true)
	rtest.OK(t, err)

	var snapshotGroupList []SnapshotGroup
	rtest.OK(t, json.Unmarshal(buf.Bytes(), &snapshotGroupList))

	rtest.Assert(t, len(snapshotGroupList) == 1, "expected 1 group, got %d", len(snapshotGroupList))
	snapshots := snapshotGroupList[0].Snapshots
	rtest.Assert(t, len(snapshots) == 3, "expected 3 snapshots, got %d", len(snapshots))
	rtest.Assert(t, snapshots[0].Time.Equal(now.Add(2*time.Hour)), "order should be preserved as given")
	rtest.Assert(t, snapshots[1].Time.Equal(now), "order should be preserved as given")
	rtest.Assert(t, snapshots[2].Time.Equal(now.Add(1*time.Hour)), "order should be preserved as given")
}

// TestSnapshotsPrintSnapshotsSortsAscending verifies that PrintSnapshots always sorts its
// input so that the oldest snapshot is listed first, regardless of the given order.
func TestSnapshotsPrintSnapshotsSortsAscending(t *testing.T) {
	now := time.Now()
	list := data.Snapshots{
		{Time: now.Add(2 * time.Hour), Hostname: "host1", Paths: []string{"/data"}},
		{Time: now, Hostname: "host1", Paths: []string{"/data"}},
		{Time: now.Add(1 * time.Hour), Hostname: "host1", Paths: []string{"/data"}},
	}

	var buf bytes.Buffer
	err := PrintSnapshots(&buf, list, nil, true)
	rtest.OK(t, err)

	rtest.Assert(t, list[0].Time.Equal(now), "expected oldest snapshot first, got %v", list[0].Time)
	rtest.Assert(t, list[1].Time.Equal(now.Add(1*time.Hour)), "expected second-oldest snapshot second, got %v", list[1].Time)
	rtest.Assert(t, list[2].Time.Equal(now.Add(2*time.Hour)), "expected newest snapshot last, got %v", list[2].Time)
}

// TestSnapshotsReverseFlag verifies that the --reverse flag defaults to false.
func TestSnapshotsReverseFlag(t *testing.T) {
	var opts SnapshotOptions
	opts.AddFlags(pflag.NewFlagSet("snapshots", pflag.ContinueOnError))
	rtest.Equals(t, false, opts.Reverse)
}
