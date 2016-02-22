package restic_test

import (
	"testing"

	"restic"
	. "restic/test"
)

func TestCache(t *testing.T) {
	repo := SetupRepo()
	defer TeardownRepo(repo)

	_, err := restic.NewCache(repo, "")
	OK(t, err)

	arch := restic.NewArchiver(repo)

	// archive some files, this should automatically cache all blobs from the snapshot
	_, _, err = arch.Snapshot(nil, []string{BenchArchiveDirectory}, nil, "")

	// TODO: test caching index
}
