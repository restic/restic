package restic_test

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

func TestFindLatestSnapshot(t *testing.T) {
	repo := repository.TestRepository(t)
	restic.TestCreateSnapshot(t, repo, parseTimeUTC("2015-05-05 05:05:05"), 1)
	restic.TestCreateSnapshot(t, repo, parseTimeUTC("2017-07-07 07:07:07"), 1)
	latestSnapshot := restic.TestCreateSnapshot(t, repo, parseTimeUTC("2019-09-09 09:09:09"), 1)

	f := restic.SnapshotFilter{Hosts: []string{"foo"}}
	sn, _, err := f.FindLatest(context.TODO(), repo, repo, "latest")
	if err != nil {
		t.Fatalf("FindLatest returned error: %v", err)
	}

	if *sn.ID() != *latestSnapshot.ID() {
		t.Errorf("FindLatest returned wrong snapshot ID: %v", *sn.ID())
	}
}

func TestFindLatestSnapshotWithMaxTimestamp(t *testing.T) {
	repo := repository.TestRepository(t)
	restic.TestCreateSnapshot(t, repo, parseTimeUTC("2015-05-05 05:05:05"), 1)
	desiredSnapshot := restic.TestCreateSnapshot(t, repo, parseTimeUTC("2017-07-07 07:07:07"), 1)
	restic.TestCreateSnapshot(t, repo, parseTimeUTC("2019-09-09 09:09:09"), 1)

	sn, _, err := (&restic.SnapshotFilter{
		Hosts:          []string{"foo"},
		TimestampLimit: parseTimeUTC("2018-08-08 08:08:08"),
	}).FindLatest(context.TODO(), repo, repo, "latest")
	if err != nil {
		t.Fatalf("FindLatest returned error: %v", err)
	}

	if *sn.ID() != *desiredSnapshot.ID() {
		t.Errorf("FindLatest returned wrong snapshot ID: %v", *sn.ID())
	}
}

func TestFindLatestWithSubpath(t *testing.T) {
	repo := repository.TestRepository(t)
	restic.TestCreateSnapshot(t, repo, parseTimeUTC("2015-05-05 05:05:05"), 1)
	desiredSnapshot := restic.TestCreateSnapshot(t, repo, parseTimeUTC("2017-07-07 07:07:07"), 1)

	for _, exp := range []struct {
		query     string
		subfolder string
	}{
		{"latest", ""},
		{"latest:subfolder", "subfolder"},
		{desiredSnapshot.ID().Str(), ""},
		{desiredSnapshot.ID().Str() + ":subfolder", "subfolder"},
		{desiredSnapshot.ID().String(), ""},
		{desiredSnapshot.ID().String() + ":subfolder", "subfolder"},
	} {
		t.Run("", func(t *testing.T) {
			sn, subfolder, err := (&restic.SnapshotFilter{}).FindLatest(context.TODO(), repo, repo, exp.query)
			if err != nil {
				t.Fatalf("FindLatest returned error: %v", err)
			}

			test.Assert(t, *sn.ID() == *desiredSnapshot.ID(), "FindLatest returned wrong snapshot ID: %v", *sn.ID())
			test.Assert(t, subfolder == exp.subfolder, "FindLatest returned wrong path in snapshot: %v", subfolder)
		})
	}
}

func TestFindAllSubpathError(t *testing.T) {
	repo := repository.TestRepository(t)
	desiredSnapshot := restic.TestCreateSnapshot(t, repo, parseTimeUTC("2017-07-07 07:07:07"), 1)

	count := 0
	test.OK(t, (&restic.SnapshotFilter{}).FindAll(context.TODO(), repo, repo,
		[]string{"latest:subfolder", desiredSnapshot.ID().Str() + ":subfolder"},
		func(id string, sn *restic.Snapshot, err error) error {
			if err == restic.ErrInvalidSnapshotSyntax {
				count++
				return nil
			}
			return err
		}))
	test.Assert(t, count == 2, "unexpected number of subfolder errors: %v, wanted %v", count, 2)
}
