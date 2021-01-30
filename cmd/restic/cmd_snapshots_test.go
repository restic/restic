package main

import (
	"strings"
	"testing"

	rtest "github.com/restic/restic/internal/test"
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
