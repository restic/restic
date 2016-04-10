package restic_test

import (
	"restic"
	"restic/checker"
	"restic/repository"
	"testing"
	"time"
)

var testSnapshotTime = time.Unix(1460289341, 207401672)

func TestCreateSnapshot(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	restic.TestCreateSnapshot(t, repo, testSnapshotTime)

	snapshots, err := restic.LoadAllSnapshots(repo)
	if err != nil {
		t.Fatal(err)
	}

	if len(snapshots) != 1 {
		t.Fatalf("got %d snapshots, expected %d", len(snapshots), 1)
	}

	sn := snapshots[0]
	if sn.Time != testSnapshotTime {
		t.Fatalf("got timestamp %v, expected %v", sn.Time, testSnapshotTime)
	}

	if sn.Tree == nil {
		t.Fatalf("tree id is nil")
	}

	if sn.Tree.IsNull() {
		t.Fatalf("snapshot has zero tree ID")
	}

	chkr := checker.New(repo)

	hints, errs := chkr.LoadIndex()
	if len(errs) != 0 {
		t.Fatalf("errors loading index: %v", errs)
	}

	if len(hints) != 0 {
		t.Fatalf("errors loading index: %v", hints)
	}

	done := make(chan struct{})
	defer close(done)
	errChan := make(chan error)
	go chkr.Structure(errChan, done)

	for err := range errChan {
		t.Error(err)
	}
}
