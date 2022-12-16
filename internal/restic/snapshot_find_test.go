package restic_test

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

func TestFindLatestSnapshot(t *testing.T) {
	repo := repository.TestRepository(t)
	restic.TestCreateSnapshot(t, repo, parseTimeUTC("2015-05-05 05:05:05"), 1, 0)
	restic.TestCreateSnapshot(t, repo, parseTimeUTC("2017-07-07 07:07:07"), 1, 0)
	latestSnapshot := restic.TestCreateSnapshot(t, repo, parseTimeUTC("2019-09-09 09:09:09"), 1, 0)

	sn, err := restic.FindFilteredSnapshot(context.TODO(), repo.Backend(), repo, []string{"foo"}, []restic.TagList{}, []string{}, nil, "latest")
	if err != nil {
		t.Fatalf("FindLatestSnapshot returned error: %v", err)
	}

	if *sn.ID() != *latestSnapshot.ID() {
		t.Errorf("FindLatestSnapshot returned wrong snapshot ID: %v", *sn.ID())
	}
}

func TestFindLatestSnapshotWithMaxTimestamp(t *testing.T) {
	repo := repository.TestRepository(t)
	restic.TestCreateSnapshot(t, repo, parseTimeUTC("2015-05-05 05:05:05"), 1, 0)
	desiredSnapshot := restic.TestCreateSnapshot(t, repo, parseTimeUTC("2017-07-07 07:07:07"), 1, 0)
	restic.TestCreateSnapshot(t, repo, parseTimeUTC("2019-09-09 09:09:09"), 1, 0)

	maxTimestamp := parseTimeUTC("2018-08-08 08:08:08")

	sn, err := restic.FindFilteredSnapshot(context.TODO(), repo.Backend(), repo, []string{"foo"}, []restic.TagList{}, []string{}, &maxTimestamp, "latest")
	if err != nil {
		t.Fatalf("FindLatestSnapshot returned error: %v", err)
	}

	if *sn.ID() != *desiredSnapshot.ID() {
		t.Errorf("FindLatestSnapshot returned wrong snapshot ID: %v", *sn.ID())
	}
}
