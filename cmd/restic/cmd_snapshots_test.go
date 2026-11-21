package main

import (
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui"
)

// Regression test for #2979: no snapshots should print as [], not null.
func TestEmptySnapshotGroupJSON(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		var w strings.Builder
		err := printSnapshotGroupJSON(&w, nil, grouped)
		rtest.OK(t, err)

		rtest.Equals(t, "[]", strings.TrimSpace(w.String()))
	}
}

func testSnapshotWithSummary(t *testing.T) *data.Snapshot {
	sn := &data.Snapshot{
		Time:  time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
		Paths: []string{"/source"},
		Summary: &data.SnapshotSummary{
			TotalBytesProcessed: 3000,
			DataAdded:           2000,
			DataAddedPacked:     1000,
			TotalFilesProcessed: 42,
		},
	}
	data.TestSetSnapshotID(t, sn, restic.Hash([]byte("snapshot")))
	return sn
}

func renderSnapshotView(t *testing.T, sn *data.Snapshot, view ...string) string {
	opts := SnapshotOptions{View: view}
	rtest.OK(t, opts.Finalize())
	var w strings.Builder
	rtest.OK(t, printSnapshotsColumns(&w, data.Snapshots{sn}, nil, opts.columns, opts.compactLayout))
	return w.String()
}

func TestSnapshotViews(t *testing.T) {
	sn := testSnapshotWithSummary(t)

	out := renderSnapshotView(t, sn)
	// default view is unchanged: id, time, host, tags, paths only
	for _, s := range []string{"Paths", "/source"} {
		rtest.Assert(t, strings.Contains(out, s), "default view should contain %q:\n%s", s, out)
	}
	for _, s := range []string{"Added", "Stored", ui.FormatBytes(2000)} {
		rtest.Assert(t, !strings.Contains(out, s), "default view should not contain %q:\n%s", s, out)
	}

	out = renderSnapshotView(t, sn, "compact")
	rtest.Assert(t, !strings.Contains(out, "Paths") && !strings.Contains(out, "Added"), "compact view shows paths or added:\n%s", out)
	rtest.Assert(t, strings.Contains(out, "Stored"), "compact view should show stored:\n%s", out)

	out = renderSnapshotView(t, sn, "growth")
	for _, s := range []string{"Size", "Added", "Stored", "Files", "42"} {
		rtest.Assert(t, strings.Contains(out, s), "growth view should contain %q:\n%s", s, out)
	}
	rtest.Assert(t, !strings.Contains(out, "Paths") && !strings.Contains(out, "Tags"), "growth view shows paths or tags:\n%s", out)
}

func TestSnapshotViewMerging(t *testing.T) {
	sn := testSnapshotWithSummary(t)

	// a view and a column are merged into one list
	out := renderSnapshotView(t, sn, "compact", "paths")
	rtest.Assert(t, strings.Contains(out, "Stored") && strings.Contains(out, "/source"), "compact,paths incomplete:\n%s", out)
	rtest.Assert(t, !strings.Contains(out, "Added"), "compact,paths shows added:\n%s", out)

	// order is preserved and duplicates are dropped
	columns, _, err := resolveView([]string{"size", "Compact", "id"})
	rtest.OK(t, err)
	rtest.Equals(t, []string{"size", "id", "time", "host", "tags", "stored"}, columns)

	// listing columns drops everything else
	out = renderSnapshotView(t, sn, "id", "size")
	rtest.Assert(t, !strings.Contains(out, "Paths") && !strings.Contains(out, "Host"), "unlisted columns shown:\n%s", out)
}

func TestSnapshotViewInvalid(t *testing.T) {
	for _, opts := range []SnapshotOptions{
		{View: []string{"bogus"}},
		{View: []string{"compact", "bogus"}},
		{Compact: true, View: []string{"growth"}},
	} {
		rtest.Assert(t, opts.Finalize() != nil, "expected error for %+v", opts)
	}
}

func TestPrintSnapshotsCompact(t *testing.T) {
	var w strings.Builder
	rtest.OK(t, PrintSnapshots(&w, data.Snapshots{testSnapshotWithSummary(t)}, nil, true))
	rtest.Assert(t, !strings.Contains(w.String(), "Paths"), "compact output shows paths:\n%s", w.String())
}
