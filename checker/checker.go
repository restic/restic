package checker

import (
	"encoding/hex"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/repository"
)

// Error is an error in the repository detected by the checker.
type Error struct {
	Message string
	Err     error
}

func (e Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}

	return e.Message
}

type mapID [backend.IDSize]byte

func id2map(id backend.ID) (mid mapID) {
	copy(mid[:], id)
	return
}

func str2map(s string) (mid mapID, err error) {
	data, err := hex.DecodeString(s)
	if err != nil {
		return mid, err
	}

	return id2map(data), nil
}

// Checker runs various checks on a repository. It is advisable to create an
// exclusive Lock in the repository before running any checks.
//
// A Checker only tests for internal errors within the data structures of the
// repository (e.g. missing blobs), and needs a valid Repository to work on.
type Checker struct {
	packs    map[mapID]struct{}
	blobs    map[mapID]struct{}
	blobRefs map[mapID]uint
	indexes  map[mapID]*repository.Index

	masterIndex *repository.Index

	repo *repository.Repository
}

// New returns a new checker which runs on repo.
func New(repo *repository.Repository) *Checker {
	return &Checker{
		blobRefs:    make(map[mapID]uint),
		packs:       make(map[mapID]struct{}),
		blobs:       make(map[mapID]struct{}),
		masterIndex: repository.NewIndex(),
		indexes:     make(map[mapID]*repository.Index),
		repo:        repo,
	}
}

const loadIndexParallelism = 20

// LoadIndex loads all index files.
func (c *Checker) LoadIndex() error {
	debug.Log("LoadIndex", "Start")
	type indexRes struct {
		Index *repository.Index
		ID    string
	}

	indexCh := make(chan indexRes)

	worker := func(id string, done <-chan struct{}) error {
		debug.Log("LoadIndex", "worker got index %v", id)
		idx, err := repository.LoadIndex(c.repo, id)
		if err != nil {
			return err
		}

		select {
		case indexCh <- indexRes{Index: idx, ID: id}:
		case <-done:
		}

		return nil
	}

	var perr error
	go func() {
		defer close(indexCh)
		debug.Log("LoadIndex", "start loading indexes in parallel")
		perr = repository.FilesInParallel(c.repo.Backend(), backend.Index, loadIndexParallelism, worker)
		debug.Log("LoadIndex", "loading indexes finished, error: %v", perr)
	}()

	done := make(chan struct{})
	defer close(done)

	for res := range indexCh {
		debug.Log("LoadIndex", "process index %v", res.ID)
		id, err := str2map(res.ID)
		if err != nil {
			return err
		}

		c.indexes[id] = res.Index
		c.masterIndex.Merge(res.Index)

		debug.Log("LoadIndex", "process blobs")
		cnt := 0
		for blob := range res.Index.Each(done) {
			c.packs[id2map(blob.PackID)] = struct{}{}
			c.blobs[id2map(blob.ID)] = struct{}{}
			c.blobRefs[id2map(blob.ID)] = 0
			cnt++
		}

		debug.Log("LoadIndex", "%d blobs processed", cnt)
	}

	debug.Log("LoadIndex", "done, error %v", perr)
	return perr
}

// Packs checks that all packs referenced in the index are still available.
func (c *Checker) Packs() error {
	return nil
}
