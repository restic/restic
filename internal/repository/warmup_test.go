package repository

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/mock"
	"github.com/restic/restic/internal/restic"
)

func TestWarmupRepository(t *testing.T) {
	warmupCalls := [][]backend.Handle{}
	warmupWaitCalls := [][]backend.Handle{}
	simulateWarmingUp := false

	be := mock.NewBackend()
	be.WarmupFn = func(ctx context.Context, handles []backend.Handle) ([]backend.Handle, error) {
		warmupCalls = append(warmupCalls, handles)
		if simulateWarmingUp {
			return handles, nil
		}
		return []backend.Handle{}, nil
	}
	be.WarmupWaitFn = func(ctx context.Context, handles []backend.Handle) error {
		warmupWaitCalls = append(warmupWaitCalls, handles)
		return nil
	}

	repo, _ := New(be, Options{})

	id1, _ := restic.ParseID("1111111111111111111111111111111111111111111111111111111111111111")
	id2, _ := restic.ParseID("2222222222222222222222222222222222222222222222222222222222222222")
	id3, _ := restic.ParseID("3333333333333333333333333333333333333333333333333333333333333333")
	job, err := repo.StartWarmup(context.TODO(), restic.NewIDSet(id1, id2))
	if err != nil {
		t.Fatalf("error when starting warmup: %v", err)
	}
	if len(warmupCalls) != 1 {
		t.Fatalf("expected %d calls to warmup, got %d", 1, len(warmupCalls))
	}
	if len(warmupCalls[0]) != 2 {
		t.Fatalf("expected warmup on %d handles, got %d", 2, len(warmupCalls[0]))
	}
	if job.HandleCount() != 0 {
		t.Fatalf("expected all files to be warm, got %d cold", job.HandleCount())
	}

	simulateWarmingUp = true
	job, err = repo.StartWarmup(context.TODO(), restic.NewIDSet(id3))
	if err != nil {
		t.Fatalf("error when starting warmup: %v", err)
	}
	if len(warmupCalls) != 2 {
		t.Fatalf("expected %d calls to warmup, got %d", 2, len(warmupCalls))
	}
	if len(warmupCalls[1]) != 1 {
		t.Fatalf("expected warmup on %d handles, got %d", 1, len(warmupCalls[1]))
	}
	if job.HandleCount() != 1 {
		t.Fatalf("expected %d file to be warming up, got %d", 1, job.HandleCount())
	}

	if err := job.Wait(context.TODO()); err != nil {
		t.Fatalf("error when waiting warmup: %v", err)
	}
	if len(warmupWaitCalls) != 1 {
		t.Fatalf("expected %d calls to warmupWait, got %d", 1, len(warmupCalls))
	}
	if len(warmupWaitCalls[0]) != 1 {
		t.Fatalf("expected warmupWait to be called with %d handles, got %d", 1, len(warmupWaitCalls[0]))
	}
}
