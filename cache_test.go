package restic_test

import (
	"encoding/json"
	"testing"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
)

func TestCache(t *testing.T) {
	server := setupBackend(t)
	defer teardownBackend(t, server)
	key := setupKey(t, server, "geheim")
	server.SetKey(key)

	cache, err := restic.NewCache(server)
	ok(t, err)

	arch, err := restic.NewArchiver(server)
	ok(t, err)

	// archive some files, this should automatically cache all blobs from the snapshot
	_, id, err := arch.Snapshot(nil, []string{*benchArchiveDirectory}, nil)

	// try to load map from cache
	rd, err := cache.Load(backend.Snapshot, "blobs", id)
	ok(t, err)

	dec := json.NewDecoder(rd)

	m := &restic.Map{}
	err = dec.Decode(m)
	ok(t, err)

	// remove cached blob list
	ok(t, cache.Purge(backend.Snapshot, "blobs", id))

	// load map from cache again, this should fail
	rd, err = cache.Load(backend.Snapshot, "blobs", id)
	assert(t, err != nil, "Expected failure did not occur")

	// recreate cached blob list
	err = cache.RefreshSnapshots(server, nil)
	ok(t, err)

	// load map from cache again
	rd, err = cache.Load(backend.Snapshot, "blobs", id)
	ok(t, err)

	dec = json.NewDecoder(rd)

	m2 := &restic.Map{}
	err = dec.Decode(m2)
	ok(t, err)

	// compare maps
	assert(t, m.Equals(m2), "Maps are not equal")
}
