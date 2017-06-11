package restic

// CachedTree is a cached tree for a snapshot.
type CachedTree struct {
	Path string `json:"path"`
	ID   ID     `json:"id"`
	Tree *Tree  `json:"tree"`
}

// SnapshotWriter adds trees of a snapshot to the cache.
type SnapshotWriter interface {
	Add(path string, id ID, tree *Tree) error
	Close() error
}

// SnapshotReader reads back data from a cached snapshot.
type SnapshotReader interface {
	Next() (*CachedTree, error)
	Close() error
}

// Cache manages a local cache.
type Cache interface {
	// LoadSnapshot returns the snapshot from the cache.
	LoadSnapshot(ID) (*Snapshot, SnapshotReader, error)

	// NewSnapshotWriter adds a new snapshot to the cache. The returned
	// SnapshotWriter must be closed after the last tree has been added.
	NewSnapshotWriter(ID, *Snapshot) (SnapshotWriter, error)
}
