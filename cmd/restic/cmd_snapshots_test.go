package main

import (
	"bytes"
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
