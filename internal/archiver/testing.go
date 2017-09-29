package archiver

import (
	"context"
	"testing"
	"time"

	"github.com/restic/restic/internal/restic"
)

// TestSnapshot creates a new snapshot of path.
func TestSnapshot(t testing.TB, repo restic.Repository, path string, parent *restic.ID) *restic.Snapshot {
	arch := New(repo)
	sn, _, _, err := arch.Snapshot(context.TODO(), nil, []string{path}, []string{"test"}, "localhost", parent, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return sn
}
