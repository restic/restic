package rofs

import (
	"context"
	"testing"
	"testing/fstest"
	"time"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

func TestROFs(t *testing.T) {
	repo := repository.TestRepository(t)

	timestamp, err := time.Parse(time.RFC3339, "2024-02-25T17:21:56+01:00")
	if err != nil {
		t.Fatal(err)
	}

	restic.TestCreateSnapshot(t, repo, timestamp, 2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	root, err := New(ctx, repo, Config{})
	if err != nil {
		t.Fatal(err)
	}

	err = fstest.TestFS(root, "snapshots")
	if err != nil {
		t.Fatal(err)
	}
}
