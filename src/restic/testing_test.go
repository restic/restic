package restic_test

import (
	"restic"
	"restic/checker"
	"restic/repository"
	"testing"
	"time"
)

var testSnapshotTime = time.Unix(1460289341, 207401672)

const (
	testCreateSnapshots = 3
	testDepth           = 2
)

func TestCreateSnapshot(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	for i := 0; i < testCreateSnapshots; i++ {
		restic.TestCreateSnapshot(t, repo, testSnapshotTime.Add(time.Duration(i)*time.Second), testDepth)
	}

	snapshots, err := restic.LoadAllSnapshots(repo)
	if err != nil {
		t.Fatal(err)
	}

	if len(snapshots) != testCreateSnapshots {
		t.Fatalf("got %d snapshots, expected %d", len(snapshots), 1)
	}

	sn := snapshots[0]
	if sn.Time.Before(testSnapshotTime) || sn.Time.After(testSnapshotTime.Add(testCreateSnapshots*time.Second)) {
		t.Fatalf("timestamp %v is outside of the allowed time range", sn.Time)
	}

	if sn.Tree == nil {
		t.Fatalf("tree id is nil")
	}

	if sn.Tree.IsNull() {
		t.Fatalf("snapshot has zero tree ID")
	}

	checker.TestCheckRepo(t, repo)
}
