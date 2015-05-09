package restic_test

import (
	"testing"

	"github.com/restic/restic"
	. "github.com/restic/restic/test"
)

func TestCache(t *testing.T) {
	repo := SetupRepo(t)
	defer TeardownRepo(t, repo)

	_, err := restic.NewCache(repo)
	OK(t, err)

	arch := restic.NewArchiver(repo)

	// archive some files, this should automatically cache all blobs from the snapshot
	_, _, err = arch.Snapshot(nil, []string{*benchArchiveDirectory}, nil)

	// TODO: test caching index
}
