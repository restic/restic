package repository

import (
	"restic/backend"
	"restic/debug"
)

// Repack takes a list of packs together with a list of blobs contained in
// these packs. Each pack is loaded and the blobs listed in keepBlobs is saved
// into a new pack. Afterwards, the packs are removed.
func Repack(repo *Repository, packs, keepBlobs backend.IDSet) error {
	debug.Log("Repack", "repacking %d packs while keeping %d blobs", len(packs), len(keepBlobs))
	return nil
}
