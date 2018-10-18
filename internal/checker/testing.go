package checker

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
)

// TestCheckRepo runs the checker on repo.
func TestCheckRepo(t testing.TB, repo restic.Repository) {
	ui := ui.NewNilProgressUI()
	chkr := New(repo)

	hints, errs := chkr.LoadIndex(context.TODO(), ui)
	if len(errs) != 0 {
		t.Fatalf("errors loading index: %v", errs)
	}

	if len(hints) != 0 {
		t.Fatalf("errors loading index: %v", hints)
	}

	// packs
	errChan := make(chan error)
	go chkr.Packs(context.TODO(), ui, errChan)

	for err := range errChan {
		t.Error(err)
	}

	// structure
	errChan = make(chan error)
	go chkr.Structure(context.TODO(), ui, errChan)

	for err := range errChan {
		t.Error(err)
	}

	// unused blobs
	blobs := chkr.UnusedBlobs()
	if len(blobs) > 0 {
		t.Errorf("unused blobs found: %v", blobs)
	}

	// read data
	errChan = make(chan error)
	go chkr.ReadData(context.TODO(), ui, errChan)

	for err := range errChan {
		t.Error(err)
	}
}
