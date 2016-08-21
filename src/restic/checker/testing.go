package checker

import (
	"restic/repository"
	"testing"
)

// TestCheckRepo runs the checker on repo.
func TestCheckRepo(t testing.TB, repo *repository.Repository) {
	chkr := New(repo)

	hints, errs := chkr.LoadIndex()
	if len(errs) != 0 {
		t.Fatalf("errors loading index: %v", errs)
	}

	if len(hints) != 0 {
		t.Fatalf("errors loading index: %v", hints)
	}

	done := make(chan struct{})
	defer close(done)

	// packs
	errChan := make(chan error)
	go chkr.Packs(errChan, done)

	for err := range errChan {
		t.Error(err)
	}

	// structure
	errChan = make(chan error)
	go chkr.Structure(errChan, done)

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
	go chkr.ReadData(nil, errChan, done)

	for err := range errChan {
		t.Error(err)
	}
}
