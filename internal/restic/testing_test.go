package restic_test

import (
	"context"
	"testing"
	"time"

	"github.com/restic/restic/internal/checker"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

var testSnapshotTime = time.Unix(1460289341, 207401672)

const (
	testCreateSnapshots = 3
	testDepth           = 2
)

func TestCreateSnapshot(t *testing.T) {
	repo := repository.TestRepository(t)
	for i := 0; i < testCreateSnapshots; i++ {
		restic.TestCreateSnapshot(t, repo, testSnapshotTime.Add(time.Duration(i)*time.Second), testDepth)
	}

	snapshots, err := restic.TestLoadAllSnapshots(context.TODO(), repo, restic.NewIDSet())
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

	checker.TestCheckRepo(t, repo, false)
}

func BenchmarkTestCreateSnapshot(t *testing.B) {
	repo := repository.TestRepository(t)

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		restic.TestCreateSnapshot(t, repo, testSnapshotTime.Add(time.Duration(i)*time.Second), testDepth)
	}
}
