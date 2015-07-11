package checker

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/repository"
)

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

func map2str(id mapID) string {
	return hex.EncodeToString(id[:])
}

func map2id(id mapID) backend.ID {
	return backend.ID(id[:])
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

const defaultParallelism = 20

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
		perr = repository.FilesInParallel(c.repo.Backend(), backend.Index, defaultParallelism, worker)
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

	c.repo.SetIndex(c.masterIndex)

	return perr
}

// PackError describes an error with a specific pack.
type PackError struct {
	ID backend.ID
	error
}

func (e PackError) Error() string {
	return "pack " + e.ID.String() + ": " + e.error.Error()
}

func packIDTester(repo *repository.Repository, inChan <-chan mapID, errChan chan<- error, wg *sync.WaitGroup, done <-chan struct{}) {
	debug.Log("Checker.testPackID", "worker start")
	defer debug.Log("Checker.testPackID", "worker done")

	defer wg.Done()

	for id := range inChan {
		ok, err := repo.Backend().Test(backend.Data, map2str(id))
		if err != nil {
			err = PackError{map2id(id), err}
		} else {
			if !ok {
				err = PackError{map2id(id), errors.New("does not exist")}
			}
		}

		if err != nil {
			debug.Log("Checker.testPackID", "error checking for pack %s: %v", map2id(id).Str(), err)
			select {
			case <-done:
				return
			case errChan <- err:
			}

			continue
		}

		debug.Log("Checker.testPackID", "pack %s exists", map2id(id).Str())
	}
}

func collectErrors(in <-chan error, out chan<- []error, done <-chan struct{}) {
	var errs []error

outer:
	for {
		select {
		case err, ok := <-in:
			if !ok {
				break outer
			}
			errs = append(errs, err)
		case <-done:
			break outer
		}
	}

	out <- errs
}

// Packs checks that all packs referenced in the index are still available and
// there are no packs that aren't in an index. errChan is closed after all
// packs have been checked.
func (c *Checker) Packs(errChan chan<- error, done <-chan struct{}) {
	defer close(errChan)

	debug.Log("Checker.Packs", "checking for %d packs", len(c.packs))
	seenPacks := make(map[mapID]struct{})

	var workerWG sync.WaitGroup

	IDChan := make(chan mapID)
	for i := 0; i < defaultParallelism; i++ {
		workerWG.Add(1)
		go packIDTester(c.repo, IDChan, errChan, &workerWG, done)
	}

	for id := range c.packs {
		seenPacks[id] = struct{}{}
		IDChan <- id
	}
	close(IDChan)

	debug.Log("Checker.Packs", "waiting for %d workers to terminate", defaultParallelism)
	workerWG.Wait()
	debug.Log("Checker.Packs", "workers terminated")

	for id := range c.repo.List(backend.Data, done) {
		debug.Log("Checker.Packs", "check data blob %v", id)
		if _, ok := seenPacks[id2map(id)]; !ok {
			select {
			case <-done:
				return
			case errChan <- PackError{id, errors.New("not referenced in any index")}:
			}
		}
	}
}

// Error is an error that occurred while checking a repository.
type Error struct {
	TreeID backend.ID
	BlobID backend.ID
	Err    error
}

func (e Error) Error() string {
	if e.BlobID != nil && e.TreeID != nil {
		msg := "tree " + e.TreeID.String()
		msg += ", blob " + e.BlobID.String()
		msg += ": " + e.Err.Error()
		return msg
	}

	if e.TreeID != nil {
		return "tree " + e.TreeID.String() + ": " + e.Err.Error()
	}

	return e.Err.Error()
}

func loadTreeFromSnapshot(repo *repository.Repository, id backend.ID) (backend.ID, error) {
	sn, err := restic.LoadSnapshot(repo, id)
	if err != nil {
		debug.Log("Checker.loadTreeFromSnapshot", "error loading snapshot %v: %v", id.Str(), err)
		return nil, err
	}

	if sn.Tree == nil {
		debug.Log("Checker.loadTreeFromSnapshot", "snapshot %v has no tree", id.Str())
		return nil, fmt.Errorf("snapshot %v has no tree", id)
	}

	return sn.Tree, nil
}

// Structure checks that for all snapshots all referenced blobs are available
// in the index. errChan is closed after all trees have been traversed.
func (c *Checker) Structure(errChan chan<- error, done <-chan struct{}) {
	defer close(errChan)

	var todo backend.IDs

	for id := range c.repo.List(backend.Snapshot, done) {
		debug.Log("Checker.Snaphots", "check snapshot %v", id.Str())

		treeID, err := loadTreeFromSnapshot(c.repo, id)
		if err != nil {
			select {
			case <-done:
				return
			case errChan <- err:
			}
			continue
		}

		debug.Log("Checker.Snaphots", "snapshot %v has tree %v", id.Str(), treeID.Str())
		todo = append(todo, treeID)
	}

	for _, err := range c.trees(todo) {
		select {
		case <-done:
			return
		case errChan <- err:
		}
	}
}

func (c *Checker) trees(treeIDs backend.IDs) (errs []error) {
	treesChecked := make(map[mapID]struct{})

	for len(treeIDs) > 0 {
		id := treeIDs[0]
		treeIDs = treeIDs[1:]

		c.blobRefs[id2map(id)]++
		debug.Log("Checker.trees", "tree %v refcount %d", id.Str(), c.blobRefs[id2map(id)])

		if _, ok := treesChecked[id2map(id)]; ok {
			debug.Log("Checker.trees", "tree %v already checked", id.Str())
			continue
		}

		debug.Log("Checker.trees", "check tree %v", id.Str())

		if _, ok := c.blobs[id2map(id)]; !ok {
			errs = append(errs, Error{TreeID: id, Err: errors.New("not found in index")})
			continue
		}

		blobs, subtrees, treeErrors := c.tree(id)
		if treeErrors != nil {
			debug.Log("Checker.trees", "error checking tree %v: %v", id.Str(), treeErrors)
			errs = append(errs, treeErrors...)
			continue
		}

		for _, blobID := range blobs {
			c.blobRefs[id2map(blobID)]++
			debug.Log("Checker.trees", "blob %v refcount %d", blobID.Str(), c.blobRefs[id2map(blobID)])

			if _, ok := c.blobs[id2map(blobID)]; !ok {
				debug.Log("Checker.trees", "tree %v references blob %v which isn't contained in index", id.Str(), blobID.Str())

				errs = append(errs, Error{TreeID: id, BlobID: blobID, Err: errors.New("not found in index")})
			}
		}

		treeIDs = append(treeIDs, subtrees...)

		treesChecked[id2map(id)] = struct{}{}
	}

	return errs
}

func (c *Checker) tree(id backend.ID) (blobs backend.IDs, subtrees backend.IDs, errs []error) {
	tree, err := restic.LoadTree(c.repo, id)
	if err != nil {
		return nil, nil, []error{Error{TreeID: id, Err: err}}
	}

	for i, node := range tree.Nodes {
		switch node.Type {
		case "file":
			blobs = append(blobs, node.Content...)
		case "dir":
			if node.Subtree == nil {
				errs = append(errs, Error{TreeID: id, Err: fmt.Errorf("node %d is dir but has no subtree", i)})
				continue
			}

			subtrees = append(subtrees, node.Subtree)
		}
	}

	return blobs, subtrees, errs
}

// UnusedBlobs returns all blobs that have never been referenced.
func (c *Checker) UnusedBlobs() (blobs backend.IDs) {
	debug.Log("Checker.UnusedBlobs", "checking %d blobs", len(c.blobs))
	for id := range c.blobs {
		if c.blobRefs[id] == 0 {
			debug.Log("Checker.UnusedBlobs", "blob %v not not referenced", map2id(id).Str())
			blobs = append(blobs, map2id(id))
		}
	}

	return blobs
}
