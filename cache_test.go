package restic_test

import (
	"encoding/json"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	. "github.com/restic/restic/test"
)

func TestCache(t *testing.T) {
	server := SetupBackend(t)
	defer TeardownBackend(t, server)
	key := SetupKey(t, server, "geheim")
	server.SetKey(key)

	cache, err := restic.NewCache(server)
	OK(t, err)

	arch, err := restic.NewArchiver(server)
	OK(t, err)

	// archive some files, this should automatically cache all blobs from the snapshot
	_, id, err := arch.Snapshot(nil, []string{*benchArchiveDirectory}, nil)

	// try to load map from cache
	rd, err := cache.Load(backend.Snapshot, "blobs", id)
	OK(t, err)

	dec := json.NewDecoder(rd)

	m := &restic.Map{}
	err = dec.Decode(m)
	OK(t, err)

	// remove cached blob list
	OK(t, cache.Purge(backend.Snapshot, "blobs", id))

	// load map from cache again, this should fail
	rd, err = cache.Load(backend.Snapshot, "blobs", id)
	Assert(t, err != nil, "Expected failure did not occur")

	// recreate cached blob list
	err = cache.RefreshSnapshots(server, nil)
	OK(t, err)

	// load map from cache again
	rd, err = cache.Load(backend.Snapshot, "blobs", id)
	OK(t, err)

	dec = json.NewDecoder(rd)

	m2 := &restic.Map{}
	err = dec.Decode(m2)
	OK(t, err)

	// compare maps
	Assert(t, m.Equals(m2), "Maps are not equal")
}
