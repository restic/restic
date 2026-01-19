package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

// Note: All test functions starting with "TestSnapshots", to run all the tests in this file:
// go test -v -run TestSnapshots ./cmd/restic/...

// Regression test for #2979: no snapshots should print as [], not null.
func TestSnapshotsEmptySnapshotGroupJSON(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		var w strings.Builder
		err := printSnapshotGroupJSON(&w, nil, grouped, "asc")
		rtest.OK(t, err)

		rtest.Equals(t, "[]", strings.TrimSpace(w.String()))
	}
}

// TestSnapshotsSortByTimeAsc verifies that snapshots are sorted in ascending order (oldest first).
func TestSnapshotsSortByTimeAsc(t *testing.T) {
	// Create test snapshots with different times
	now := time.Now()
	snapshots := []Snapshot{
		{
			Snapshot: &data.Snapshot{Time: now.Add(2 * time.Hour)},
			ID:       &restic.ID{},
			ShortID:  "snap3",
		},
		{
			Snapshot: &data.Snapshot{Time: now},
			ID:       &restic.ID{},
			ShortID:  "snap1",
		},
		{
			Snapshot: &data.Snapshot{Time: now.Add(1 * time.Hour)},
			ID:       &restic.ID{},
			ShortID:  "snap2",
		},
	}

	// Sort in ascending order
	sortSnapshotsByTime(snapshots, "asc")

	// Verify snapshots are sorted oldest first
	rtest.Equals(t, "snap1", snapshots[0].ShortID)
	rtest.Equals(t, "snap2", snapshots[1].ShortID)
	rtest.Equals(t, "snap3", snapshots[2].ShortID)
}

// TestSnapshotsSortByTimeDesc verifies that snapshots are sorted in descending order (newest first).
func TestSnapshotsSortByTimeDesc(t *testing.T) {
	// Create test snapshots with different times
	now := time.Now()
	snapshots := []Snapshot{
		{
			Snapshot: &data.Snapshot{Time: now},
			ID:       &restic.ID{},
			ShortID:  "snap1",
		},
		{
			Snapshot: &data.Snapshot{Time: now.Add(2 * time.Hour)},
			ID:       &restic.ID{},
			ShortID:  "snap3",
		},
		{
			Snapshot: &data.Snapshot{Time: now.Add(1 * time.Hour)},
			ID:       &restic.ID{},
			ShortID:  "snap2",
		},
	}

	// Sort in descending order
	sortSnapshotsByTime(snapshots, "desc")

	// Verify snapshots are sorted newest first
	rtest.Equals(t, "snap3", snapshots[0].ShortID)
	rtest.Equals(t, "snap2", snapshots[1].ShortID)
	rtest.Equals(t, "snap1", snapshots[2].ShortID)
}

// TestSnapshotsPrintSnapshotGroupJSONSortAsc verifies JSON output is sorted in ascending order.
func TestSnapshotsPrintSnapshotGroupJSONSortAsc(t *testing.T) {
	now := time.Now()
	snapshotGroups := map[string]data.Snapshots{
		"{}": {
			&data.Snapshot{Time: now.Add(2 * time.Hour), Hostname: "host1", Paths: []string{"/data"}},
			&data.Snapshot{Time: now, Hostname: "host1", Paths: []string{"/data"}},
			&data.Snapshot{Time: now.Add(1 * time.Hour), Hostname: "host1", Paths: []string{"/data"}},
		},
	}

	var buf bytes.Buffer
	err := printSnapshotGroupJSON(&buf, snapshotGroups, false, "asc")
	rtest.OK(t, err)

	var snapshots []Snapshot
	rtest.OK(t, json.Unmarshal(buf.Bytes(), &snapshots))

	// Verify snapshots are sorted oldest first
	rtest.Assert(t, len(snapshots) == 3, "expected 3 snapshots, got %d", len(snapshots))
	rtest.Assert(t, snapshots[0].Time.Before(snapshots[1].Time), "first snapshot should be before second")
	rtest.Assert(t, snapshots[1].Time.Before(snapshots[2].Time), "second snapshot should be before third")
}

// TestSnapshotsPrintSnapshotGroupJSONSortDesc verifies JSON output is sorted in descending order.
func TestSnapshotsPrintSnapshotGroupJSONSortDesc(t *testing.T) {
	now := time.Now()
	snapshotGroups := map[string]data.Snapshots{
		"{}": {
			&data.Snapshot{Time: now, Hostname: "host1", Paths: []string{"/data"}},
			&data.Snapshot{Time: now.Add(2 * time.Hour), Hostname: "host1", Paths: []string{"/data"}},
			&data.Snapshot{Time: now.Add(1 * time.Hour), Hostname: "host1", Paths: []string{"/data"}},
		},
	}

	var buf bytes.Buffer
	err := printSnapshotGroupJSON(&buf, snapshotGroups, false, "desc")
	rtest.OK(t, err)

	var snapshots []Snapshot
	rtest.OK(t, json.Unmarshal(buf.Bytes(), &snapshots))

	// Verify snapshots are sorted newest first
	rtest.Assert(t, len(snapshots) == 3, "expected 3 snapshots, got %d", len(snapshots))
	rtest.Assert(t, snapshots[0].Time.After(snapshots[1].Time), "first snapshot should be after second")
	rtest.Assert(t, snapshots[1].Time.After(snapshots[2].Time), "second snapshot should be after third")
}

// TestSnapshotsPrintSnapshotGroupJSONGroupedSortAsc verifies grouped JSON output is sorted in ascending order.
func TestSnapshotsPrintSnapshotGroupJSONGroupedSortAsc(t *testing.T) {
	now := time.Now()
	snapshotGroups := map[string]data.Snapshots{
		`{"hostname":"host1","tags":null,"paths":null}`: {
			&data.Snapshot{Time: now.Add(2 * time.Hour), Hostname: "host1", Paths: []string{"/data"}},
			&data.Snapshot{Time: now, Hostname: "host1", Paths: []string{"/data"}},
			&data.Snapshot{Time: now.Add(1 * time.Hour), Hostname: "host1", Paths: []string{"/data"}},
		},
	}

	var buf bytes.Buffer
	err := printSnapshotGroupJSON(&buf, snapshotGroups, true, "asc")
	rtest.OK(t, err)

	var snapshotGroupList []SnapshotGroup
	rtest.OK(t, json.Unmarshal(buf.Bytes(), &snapshotGroupList))

	// Verify we have one group with 3 snapshots
	rtest.Assert(t, len(snapshotGroupList) == 1, "expected 1 group, got %d", len(snapshotGroupList))
	rtest.Assert(t, len(snapshotGroupList[0].Snapshots) == 3, "expected 3 snapshots, got %d", len(snapshotGroupList[0].Snapshots))

	// Verify snapshots are sorted oldest first
	snapshots := snapshotGroupList[0].Snapshots
	rtest.Assert(t, snapshots[0].Time.Before(snapshots[1].Time), "first snapshot should be before second")
	rtest.Assert(t, snapshots[1].Time.Before(snapshots[2].Time), "second snapshot should be before third")
}

// TestSnapshotsPrintSnapshotGroupJSONGroupedSortDesc verifies grouped JSON output is sorted in descending order.
func TestSnapshotsPrintSnapshotGroupJSONGroupedSortDesc(t *testing.T) {
	now := time.Now()
	snapshotGroups := map[string]data.Snapshots{
		`{"hostname":"host1","tags":null,"paths":null}`: {
			&data.Snapshot{Time: now, Hostname: "host1", Paths: []string{"/data"}},
			&data.Snapshot{Time: now.Add(2 * time.Hour), Hostname: "host1", Paths: []string{"/data"}},
			&data.Snapshot{Time: now.Add(1 * time.Hour), Hostname: "host1", Paths: []string{"/data"}},
		},
	}

	var buf bytes.Buffer
	err := printSnapshotGroupJSON(&buf, snapshotGroups, true, "desc")
	rtest.OK(t, err)

	var snapshotGroupList []SnapshotGroup
	rtest.OK(t, json.Unmarshal(buf.Bytes(), &snapshotGroupList))

	// Verify we have one group with 3 snapshots
	rtest.Assert(t, len(snapshotGroupList) == 1, "expected 1 group, got %d", len(snapshotGroupList))
	rtest.Assert(t, len(snapshotGroupList[0].Snapshots) == 3, "expected 3 snapshots, got %d", len(snapshotGroupList[0].Snapshots))

	// Verify snapshots are sorted newest first
	snapshots := snapshotGroupList[0].Snapshots
	rtest.Assert(t, snapshots[0].Time.After(snapshots[1].Time), "first snapshot should be after second")
	rtest.Assert(t, snapshots[1].Time.After(snapshots[2].Time), "second snapshot should be after third")
}
