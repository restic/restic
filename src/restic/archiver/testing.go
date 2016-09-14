package archiver

import (
	"restic"
	"testing"
)

// TestSnapshot creates a new snapshot of path.
func TestSnapshot(t testing.TB, repo restic.Repository, path string, parent *restic.ID) *restic.Snapshot {
	arch := New(repo)
	sn, _, err := arch.Snapshot(nil, []string{path}, []string{"test"}, parent)
	if err != nil {
		t.Fatal(err)
	}
	return sn
}
