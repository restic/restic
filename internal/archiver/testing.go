package archiver

import (
	"context"
	"testing"

	"github.com/restic/restic/internal"
)

// TestSnapshot creates a new snapshot of path.
func TestSnapshot(t testing.TB, repo restic.Repository, path string, parent *restic.ID) *restic.Snapshot {
	arch := New(repo)
	sn, _, err := arch.Snapshot(context.TODO(), nil, []string{path}, []string{"test"}, "localhost", parent)
	if err != nil {
		t.Fatal(err)
	}
	return sn
}
