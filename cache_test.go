package restic_test

import (
	"testing"

	"github.com/restic/restic"
	. "github.com/restic/restic/test"
)

func TestCache(t *testing.T) {
	server := SetupBackend(t)
	defer TeardownBackend(t, server)
	key := SetupKey(t, server, "geheim")
	server.SetKey(key)

	_, err := restic.NewCache(server)
	OK(t, err)

	arch, err := restic.NewArchiver(server)
	OK(t, err)

	// archive some files, this should automatically cache all blobs from the snapshot
	_, _, err = arch.Snapshot(nil, []string{*benchArchiveDirectory}, nil)

	// TODO: test caching index
}
