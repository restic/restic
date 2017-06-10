package restic

import "context"

// Cache manages a local cache.
type Cache interface {
	// Snapshot returns the snapshot from the cache.
	Snapshot(id ID) (*Snapshot, error)

	// WalkSnapshot sends the trees of the snapshot (in depths-first order) to the channel ch.
	WalkSnapshot(ctx context.Context, id ID, ch chan<- Tree) error
}
