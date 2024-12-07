package repository

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/mock"
	"github.com/restic/restic/internal/restic"
)

func TestWarmupRepository(t *testing.T) {
	warmupCalls := []backend.Handle{}
	warmupWaitCalls := []backend.Handle{}
	isWarm := true

	be := mock.NewBackend()
	be.WarmupFn = func(ctx context.Context, h backend.Handle) (bool, error) {
		warmupCalls = append(warmupCalls, h)
		return isWarm, nil
	}
	be.WarmupWaitFn = func(ctx context.Context, h backend.Handle) error {
		warmupWaitCalls = append(warmupWaitCalls, h)
		return nil
	}

	repo, _ := New(be, Options{})
	packsWarmer := NewPacksWarmer(repo)

	id1, _ := restic.ParseID("1111111111111111111111111111111111111111111111111111111111111111")
	id2, _ := restic.ParseID("2222222222222222222222222222222222222222222222222222222222222222")
	id3, _ := restic.ParseID("3333333333333333333333333333333333333333333333333333333333333333")
	err := packsWarmer.StartWarmup(context.TODO(), restic.IDs{id1, id2})
	if err != nil {
		t.Fatalf("error when starting warmup: %v", err)
	}
	if len(warmupCalls) != 2 {
		t.Fatalf("expected 2 calls to warmup, got %d", len(warmupCalls))
	}

	err = packsWarmer.Wait(context.TODO(), id1)
	if err != nil {
		t.Fatalf("error when waiting for warmup: %v", err)
	}
	if len(warmupWaitCalls) != 0 {
		t.Fatal("WarmupWait was called on a warm file")
	}

	isWarm = false
	err = packsWarmer.StartWarmup(context.TODO(), restic.IDs{id3})
	if err != nil {
		t.Fatalf("error when adding element to warmup: %v", err)
	}
	if len(warmupCalls) != 3 {
		t.Fatalf("expected 3 calls to warmup, got %d", len(warmupCalls))
	}
	err = packsWarmer.Wait(context.TODO(), id3)
	if err != nil {
		t.Fatalf("error when waiting for warmup: %v", err)
	}
	if len(warmupWaitCalls) != 1 {
		t.Fatalf("expected one call to WarmupWait, got %d", len(warmupWaitCalls))
	}

}
