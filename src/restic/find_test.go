package restic

import (
	"testing"
	"time"

	"restic/repository"
)

const (
	testSnapshots = 3
	testDepth     = 2
)

var testTime = time.Unix(1469960361, 23)

func TestFindUsedBlobs(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	var snapshots []*Snapshot
	for i := 0; i < testSnapshots; i++ {
		sn := TestCreateSnapshot(t, repo, testTime.Add(time.Duration(i)*time.Second), testDepth)
		t.Logf("snapshot %v saved, tree %v", sn.ID().Str(), sn.Tree.Str())
		snapshots = append(snapshots, sn)
	}

	for _, sn := range snapshots {
		usedBlobs, err := FindUsedBlobs(repo, *sn.Tree)
		if err != nil {
			t.Errorf("FindUsedBlobs returned error: %v", err)
			continue
		}

		if len(usedBlobs) == 0 {
			t.Errorf("FindUsedBlobs returned an empty set")
			continue
		}

		t.Logf("used blobs from snapshot %v (tree %v): %v", sn.ID().Str(), sn.Tree.Str(), usedBlobs)
	}
}
