package cache

import (
	"os"
	"path/filepath"
	"restic"
	"restic/errors"
)

// SnapshotWriter adds a snapshot to the cache.
type SnapshotWriter struct {
	BlockWriter
}

// NewSnapshotWriter adds a new snapshot to the cache. The returned
// SnapshotWriter must be closed after the last tree has been added.
func (c *Cache) NewSnapshotWriter(id restic.ID, sn *restic.Snapshot) (*SnapshotWriter, error) {
	f, err := os.Create(filepath.Join(c.Path, id.String()))
	if err != nil {
		return nil, errors.Wrap(err, "Create")
	}

	sw := &SnapshotWriter{
		BlockWriter: BlockWriter{
			File: f,
			key:  c.Key,
		},
	}
	return sw, nil
}

// Tree is a cached tree in a snapshot.
type Tree struct {
	Path string       `json:"path"`
	ID   restic.ID    `json:"id"`
	Tree *restic.Tree `json:"tree"`
}

// AddTree writes a new tree to the cache file.
func (s *SnapshotWriter) AddTree(path string, id restic.ID, tree *restic.Tree) error {
	t := Tree{
		Path: path,
		ID:   id,
		Tree: tree,
	}

	return s.WriteJSON(t)
}

// SnapshotReader loads a snapshot with tree objects from the cache.
type SnapshotReader struct {
	BlockReader
}

// LoadSnapshot loads a snapshot from a cached file. The returned
// SnapshotReader must be closed.
func (c *Cache) LoadSnapshot(id restic.ID) (*restic.Snapshot, *SnapshotReader, error) {
	f, err := os.Open(filepath.Join(c.Path, id.String()))
	if err != nil {
		return nil, nil, errors.Wrap(err, "Open")
	}

	br := BlockReader{
		File: f,
		key:  c.Key,
	}

	var sn restic.Snapshot
	err = br.ReadJSON(&sn)
	if err != nil {
		return nil, nil, err
	}

	return &sn, &SnapshotReader{br}, nil
}
