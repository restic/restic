package restic_test

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

func TestFindLatestSnapshot(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	restic.TestCreateSnapshot(t, repo, parseTimeUTC("2015-05-05 05:05:05"), 1, 0)
	restic.TestCreateSnapshot(t, repo, parseTimeUTC("2017-07-07 07:07:07"), 1, 0)
	latestSnapshot := restic.TestCreateSnapshot(t, repo, parseTimeUTC("2019-09-09 09:09:09"), 1, 0)

	id, err := restic.FindLatestSnapshot(context.TODO(), repo.Backend(), repo, []string{}, []restic.TagList{}, []string{"foo"}, nil)
	if err != nil {
		t.Fatalf("FindLatestSnapshot returned error: %v", err)
	}

	if id != *latestSnapshot.ID() {
		t.Errorf("FindLatestSnapshot returned wrong snapshot ID: %v", id)
	}
}

func TestFindLatestSnapshotWithMaxTimestamp(t *testing.T) {
	repo, cleanup := repository.TestRepository(t)
	defer cleanup()

	restic.TestCreateSnapshot(t, repo, parseTimeUTC("2015-05-05 05:05:05"), 1, 0)
	desiredSnapshot := restic.TestCreateSnapshot(t, repo, parseTimeUTC("2017-07-07 07:07:07"), 1, 0)
	restic.TestCreateSnapshot(t, repo, parseTimeUTC("2019-09-09 09:09:09"), 1, 0)

	maxTimestamp := parseTimeUTC("2018-08-08 08:08:08")

	id, err := restic.FindLatestSnapshot(context.TODO(), repo.Backend(), repo, []string{}, []restic.TagList{}, []string{"foo"}, &maxTimestamp)
	if err != nil {
		t.Fatalf("FindLatestSnapshot returned error: %v", err)
	}

	if id != *desiredSnapshot.ID() {
		t.Errorf("FindLatestSnapshot returned wrong snapshot ID: %v", id)
	}
}
