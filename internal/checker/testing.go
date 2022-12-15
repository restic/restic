package checker

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/restic"
)

// TestCheckRepo runs the checker on repo.
func TestCheckRepo(t testing.TB, repo restic.Repository) {
	chkr := New(repo, true)

	hints, errs := chkr.LoadIndex(context.TODO())
	if len(errs) != 0 {
		t.Fatalf("errors loading index: %v", errs)
	}

	if len(hints) != 0 {
		t.Fatalf("errors loading index: %v", hints)
	}

	err := chkr.LoadSnapshots(context.TODO())
	if err != nil {
		t.Error(err)
	}

	// packs
	errChan := make(chan error)
	go chkr.Packs(context.TODO(), errChan)

	for err := range errChan {
		t.Error(err)
	}

	// structure
	errChan = make(chan error)
	go chkr.Structure(context.TODO(), nil, errChan)

	for err := range errChan {
		t.Error(err)
	}

	// unused blobs
	blobs := chkr.UnusedBlobs(context.TODO())
	if len(blobs) > 0 {
		t.Errorf("unused blobs found: %v", blobs)
	}

	// read data
	errChan = make(chan error)
	go chkr.ReadData(context.TODO(), errChan)

	for err := range errChan {
		t.Error(err)
	}
}
