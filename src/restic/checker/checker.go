package checker

import (
	"bytes"
	"fmt"
	"sync"

	"restic/errors"

	"restic"
	"restic/backend"
	"restic/crypto"
	"restic/debug"
	"restic/pack"
	"restic/repository"
)

// Checker runs various checks on a repository. It is advisable to create an
// exclusive Lock in the repository before running any checks.
//
// A Checker only tests for internal errors within the data structures of the
// repository (e.g. missing blobs), and needs a valid Repository to work on.
type Checker struct {
	packs    restic.IDSet
	blobs    restic.IDSet
	blobRefs struct {
		sync.Mutex
		M map[restic.ID]uint
	}
	indexes       map[restic.ID]*repository.Index
	orphanedPacks restic.IDs

	masterIndex *repository.MasterIndex

	repo restic.Repository
}

// New returns a new checker which runs on repo.
func New(repo restic.Repository) *Checker {
	c := &Checker{
		packs:       restic.NewIDSet(),
		blobs:       restic.NewIDSet(),
		masterIndex: repository.NewMasterIndex(),
		indexes:     make(map[restic.ID]*repository.Index),
		repo:        repo,
	}

	c.blobRefs.M = make(map[restic.ID]uint)

	return c
}

const defaultParallelism = 40

// ErrDuplicatePacks is returned when a pack is found in more than one index.
type ErrDuplicatePacks struct {
	PackID  restic.ID
	Indexes restic.IDSet
}

func (e ErrDuplicatePacks) Error() string {
	return fmt.Sprintf("pack %v contained in several indexes: %v", e.PackID.Str(), e.Indexes)
}

// ErrOldIndexFormat is returned when an index with the old format is
// found.
type ErrOldIndexFormat struct {
	restic.ID
}

func (err ErrOldIndexFormat) Error() string {
	return fmt.Sprintf("index %v has old format", err.ID.Str())
}

// LoadIndex loads all index files.
func (c *Checker) LoadIndex() (hints []error, errs []error) {
	debug.Log("Start")
	type indexRes struct {
		Index *repository.Index
		ID    string
	}

	indexCh := make(chan indexRes)

	worker := func(id restic.ID, done <-chan struct{}) error {
		debug.Log("worker got index %v", id)
		idx, err := repository.LoadIndexWithDecoder(c.repo, id, repository.DecodeIndex)
		if errors.Cause(err) == repository.ErrOldIndexFormat {
			debug.Log("index %v has old format", id.Str())
			hints = append(hints, ErrOldIndexFormat{id})

			idx, err = repository.LoadIndexWithDecoder(c.repo, id, repository.DecodeOldIndex)
		}

		if err != nil {
			return err
		}

		select {
		case indexCh <- indexRes{Index: idx, ID: id.String()}:
		case <-done:
		}

		return nil
	}

	var perr error
	go func() {
		defer close(indexCh)
		debug.Log("start loading indexes in parallel")
		perr = repository.FilesInParallel(c.repo.Backend(), restic.IndexFile, defaultParallelism,
			repository.ParallelWorkFuncParseID(worker))
		debug.Log("loading indexes finished, error: %v", perr)
	}()

	done := make(chan struct{})
	defer close(done)

	if perr != nil {
		errs = append(errs, perr)
		return hints, errs
	}

	packToIndex := make(map[restic.ID]restic.IDSet)

	for res := range indexCh {
		debug.Log("process index %v", res.ID)
		idxID, err := restic.ParseID(res.ID)
		if err != nil {
			errs = append(errs, errors.Errorf("unable to parse as index ID: %v", res.ID))
			continue
		}

		c.indexes[idxID] = res.Index
		c.masterIndex.Insert(res.Index)

		debug.Log("process blobs")
		cnt := 0
		for blob := range res.Index.Each(done) {
			c.packs.Insert(blob.PackID)
			c.blobs.Insert(blob.ID)
			c.blobRefs.M[blob.ID] = 0
			cnt++

			if _, ok := packToIndex[blob.PackID]; !ok {
				packToIndex[blob.PackID] = restic.NewIDSet()
			}
			packToIndex[blob.PackID].Insert(idxID)
		}

		debug.Log("%d blobs processed", cnt)
	}

	debug.Log("done, error %v", perr)

	debug.Log("checking for duplicate packs")
	for packID := range c.packs {
		debug.Log("  check pack %v: contained in %d indexes", packID.Str(), len(packToIndex[packID]))
		if len(packToIndex[packID]) > 1 {
			hints = append(hints, ErrDuplicatePacks{
				PackID:  packID,
				Indexes: packToIndex[packID],
			})
		}
	}

	c.repo.SetIndex(c.masterIndex)

	return hints, errs
}

// PackError describes an error with a specific pack.
type PackError struct {
	ID       restic.ID
	Orphaned bool
	Err      error
}

func (e PackError) Error() string {
	return "pack " + e.ID.String() + ": " + e.Err.Error()
}

func packIDTester(repo restic.Repository, inChan <-chan restic.ID, errChan chan<- error, wg *sync.WaitGroup, done <-chan struct{}) {
	debug.Log("worker start")
	defer debug.Log("worker done")

	defer wg.Done()

	for id := range inChan {
		ok, err := repo.Backend().Test(restic.DataFile, id.String())
		if err != nil {
			err = PackError{ID: id, Err: err}
		} else {
			if !ok {
				err = PackError{ID: id, Err: errors.New("does not exist")}
			}
		}

		if err != nil {
			debug.Log("error checking for pack %s: %v", id.Str(), err)
			select {
			case <-done:
				return
			case errChan <- err:
			}

			continue
		}

		debug.Log("pack %s exists", id.Str())
	}
}

// Packs checks that all packs referenced in the index are still available and
// there are no packs that aren't in an index. errChan is closed after all
// packs have been checked.
func (c *Checker) Packs(errChan chan<- error, done <-chan struct{}) {
	defer close(errChan)

	debug.Log("checking for %d packs", len(c.packs))
	seenPacks := restic.NewIDSet()

	var workerWG sync.WaitGroup

	IDChan := make(chan restic.ID)
	for i := 0; i < defaultParallelism; i++ {
		workerWG.Add(1)
		go packIDTester(c.repo, IDChan, errChan, &workerWG, done)
	}

	for id := range c.packs {
		seenPacks.Insert(id)
		IDChan <- id
	}
	close(IDChan)

	debug.Log("waiting for %d workers to terminate", defaultParallelism)
	workerWG.Wait()
	debug.Log("workers terminated")

	for id := range c.repo.List(restic.DataFile, done) {
		debug.Log("check data blob %v", id.Str())
		if !seenPacks.Has(id) {
			c.orphanedPacks = append(c.orphanedPacks, id)
			select {
			case <-done:
				return
			case errChan <- PackError{ID: id, Orphaned: true, Err: errors.New("not referenced in any index")}:
			}
		}
	}
}

// Error is an error that occurred while checking a repository.
type Error struct {
	TreeID restic.ID
	BlobID restic.ID
	Err    error
}

func (e Error) Error() string {
	if !e.BlobID.IsNull() && !e.TreeID.IsNull() {
		msg := "tree " + e.TreeID.Str()
		msg += ", blob " + e.BlobID.Str()
		msg += ": " + e.Err.Error()
		return msg
	}

	if !e.TreeID.IsNull() {
		return "tree " + e.TreeID.Str() + ": " + e.Err.Error()
	}

	return e.Err.Error()
}

func loadTreeFromSnapshot(repo restic.Repository, id restic.ID) (restic.ID, error) {
	sn, err := restic.LoadSnapshot(repo, id)
	if err != nil {
		debug.Log("error loading snapshot %v: %v", id.Str(), err)
		return restic.ID{}, err
	}

	if sn.Tree == nil {
		debug.Log("snapshot %v has no tree", id.Str())
		return restic.ID{}, errors.Errorf("snapshot %v has no tree", id)
	}

	return *sn.Tree, nil
}

// loadSnapshotTreeIDs loads all snapshots from backend and returns the tree IDs.
func loadSnapshotTreeIDs(repo restic.Repository) (restic.IDs, []error) {
	var trees struct {
		IDs restic.IDs
		sync.Mutex
	}

	var errs struct {
		errs []error
		sync.Mutex
	}

	snapshotWorker := func(strID string, done <-chan struct{}) error {
		id, err := restic.ParseID(strID)
		if err != nil {
			return err
		}

		debug.Log("load snapshot %v", id.Str())

		treeID, err := loadTreeFromSnapshot(repo, id)
		if err != nil {
			errs.Lock()
			errs.errs = append(errs.errs, err)
			errs.Unlock()
			return nil
		}

		debug.Log("snapshot %v has tree %v", id.Str(), treeID.Str())
		trees.Lock()
		trees.IDs = append(trees.IDs, treeID)
		trees.Unlock()

		return nil
	}

	err := repository.FilesInParallel(repo.Backend(), restic.SnapshotFile, defaultParallelism, snapshotWorker)
	if err != nil {
		errs.errs = append(errs.errs, err)
	}

	return trees.IDs, errs.errs
}

// TreeError collects several errors that occurred while processing a tree.
type TreeError struct {
	ID     restic.ID
	Errors []error
}

func (e TreeError) Error() string {
	return fmt.Sprintf("tree %v: %v", e.ID.Str(), e.Errors)
}

type treeJob struct {
	restic.ID
	error
	*restic.Tree
}

// loadTreeWorker loads trees from repo and sends them to out.
func loadTreeWorker(repo restic.Repository,
	in <-chan restic.ID, out chan<- treeJob,
	done <-chan struct{}, wg *sync.WaitGroup) {

	defer func() {
		debug.Log("exiting")
		wg.Done()
	}()

	var (
		inCh  = in
		outCh = out
		job   treeJob
	)

	outCh = nil
	for {
		select {
		case <-done:
			return

		case treeID, ok := <-inCh:
			if !ok {
				return
			}
			debug.Log("load tree %v", treeID.Str())

			tree, err := repo.LoadTree(treeID)
			debug.Log("load tree %v (%v) returned err: %v", tree, treeID.Str(), err)
			job = treeJob{ID: treeID, error: err, Tree: tree}
			outCh = out
			inCh = nil

		case outCh <- job:
			debug.Log("sent tree %v", job.ID.Str())
			outCh = nil
			inCh = in
		}
	}
}

// checkTreeWorker checks the trees received and sends out errors to errChan.
func (c *Checker) checkTreeWorker(in <-chan treeJob, out chan<- error, done <-chan struct{}, wg *sync.WaitGroup) {
	defer func() {
		debug.Log("exiting")
		wg.Done()
	}()

	var (
		inCh      = in
		outCh     = out
		treeError TreeError
	)

	outCh = nil
	for {
		select {
		case <-done:
			debug.Log("done channel closed, exiting")
			return

		case job, ok := <-inCh:
			if !ok {
				debug.Log("input channel closed, exiting")
				return
			}

			id := job.ID
			alreadyChecked := false
			c.blobRefs.Lock()
			if c.blobRefs.M[id] > 0 {
				alreadyChecked = true
			}
			c.blobRefs.M[id]++
			debug.Log("tree %v refcount %d", job.ID.Str(), c.blobRefs.M[id])
			c.blobRefs.Unlock()

			if alreadyChecked {
				continue
			}

			debug.Log("check tree %v (tree %v, err %v)", job.ID.Str(), job.Tree, job.error)

			var errs []error
			if job.error != nil {
				errs = append(errs, job.error)
			} else {
				errs = c.checkTree(job.ID, job.Tree)
			}

			if len(errs) > 0 {
				debug.Log("checked tree %v: %v errors", job.ID.Str(), len(errs))
				treeError = TreeError{ID: job.ID, Errors: errs}
				outCh = out
				inCh = nil
			}

		case outCh <- treeError:
			debug.Log("tree %v: sent %d errors", treeError.ID, len(treeError.Errors))
			outCh = nil
			inCh = in
		}
	}
}

func filterTrees(backlog restic.IDs, loaderChan chan<- restic.ID, in <-chan treeJob, out chan<- treeJob, done <-chan struct{}) {
	defer func() {
		debug.Log("closing output channels")
		close(loaderChan)
		close(out)
	}()

	var (
		inCh                    = in
		outCh                   = out
		loadCh                  = loaderChan
		job                     treeJob
		nextTreeID              restic.ID
		outstandingLoadTreeJobs = 0
	)

	outCh = nil
	loadCh = nil

	for {
		if loadCh == nil && len(backlog) > 0 {
			loadCh = loaderChan
			nextTreeID, backlog = backlog[0], backlog[1:]
		}

		if loadCh == nil && outCh == nil && outstandingLoadTreeJobs == 0 {
			debug.Log("backlog is empty, all channels nil, exiting")
			return
		}

		select {
		case <-done:
			return

		case loadCh <- nextTreeID:
			outstandingLoadTreeJobs++
			loadCh = nil

		case j, ok := <-inCh:
			if !ok {
				debug.Log("input channel closed")
				inCh = nil
				in = nil
				continue
			}

			outstandingLoadTreeJobs--

			debug.Log("input job tree %v", j.ID.Str())

			var err error

			if j.error != nil {
				debug.Log("received job with error: %v (tree %v, ID %v)", j.error, j.Tree, j.ID.Str())
			} else if j.Tree == nil {
				debug.Log("received job with nil tree pointer: %v (ID %v)", j.error, j.ID.Str())
				err = errors.New("tree is nil and error is nil")
			} else {
				debug.Log("subtrees for tree %v: %v", j.ID.Str(), j.Tree.Subtrees())
				for _, id := range j.Tree.Subtrees() {
					if id.IsNull() {
						// We do not need to raise this error here, it is
						// checked when the tree is checked. Just make sure
						// that we do not add any null IDs to the backlog.
						debug.Log("tree %v has nil subtree", j.ID.Str())
						continue
					}
					backlog = append(backlog, id)
				}
			}

			if err != nil {
				// send a new job with the new error instead of the old one
				j = treeJob{ID: j.ID, error: err}
			}

			job = j
			outCh = out
			inCh = nil

		case outCh <- job:
			debug.Log("tree sent to check: %v", job.ID.Str())
			outCh = nil
			inCh = in
		}
	}
}

// Structure checks that for all snapshots all referenced data blobs and
// subtrees are available in the index. errChan is closed after all trees have
// been traversed.
func (c *Checker) Structure(errChan chan<- error, done <-chan struct{}) {
	defer close(errChan)

	trees, errs := loadSnapshotTreeIDs(c.repo)
	debug.Log("need to check %d trees from snapshots, %d errs returned", len(trees), len(errs))

	for _, err := range errs {
		select {
		case <-done:
			return
		case errChan <- err:
		}
	}

	treeIDChan := make(chan restic.ID)
	treeJobChan1 := make(chan treeJob)
	treeJobChan2 := make(chan treeJob)

	var wg sync.WaitGroup
	for i := 0; i < defaultParallelism; i++ {
		wg.Add(2)
		go loadTreeWorker(c.repo, treeIDChan, treeJobChan1, done, &wg)
		go c.checkTreeWorker(treeJobChan2, errChan, done, &wg)
	}

	filterTrees(trees, treeIDChan, treeJobChan1, treeJobChan2, done)

	wg.Wait()
}

func (c *Checker) checkTree(id restic.ID, tree *restic.Tree) (errs []error) {
	debug.Log("checking tree %v", id.Str())

	var blobs []restic.ID

	for _, node := range tree.Nodes {
		switch node.Type {
		case "file":
			if node.Content == nil {
				errs = append(errs, Error{TreeID: id, Err: errors.Errorf("file %q has nil blob list", node.Name)})
			}

			for b, blobID := range node.Content {
				if blobID.IsNull() {
					errs = append(errs, Error{TreeID: id, Err: errors.Errorf("file %q blob %d has null ID", node.Name, b)})
					continue
				}
				blobs = append(blobs, blobID)
			}
		case "dir":
			if node.Subtree == nil {
				errs = append(errs, Error{TreeID: id, Err: errors.Errorf("dir node %q has no subtree", node.Name)})
				continue
			}

			if node.Subtree.IsNull() {
				errs = append(errs, Error{TreeID: id, Err: errors.Errorf("dir node %q subtree id is null", node.Name)})
				continue
			}

		case "symlink", "socket", "chardev", "dev", "fifo":
			// nothing to check

		default:
			errs = append(errs, Error{TreeID: id, Err: errors.Errorf("node %q with invalid type %q", node.Name, node.Type)})
		}

		if node.Name == "" {
			errs = append(errs, Error{TreeID: id, Err: errors.New("node with empty name")})
		}
	}

	for _, blobID := range blobs {
		c.blobRefs.Lock()
		c.blobRefs.M[blobID]++
		debug.Log("blob %v refcount %d", blobID.Str(), c.blobRefs.M[blobID])
		c.blobRefs.Unlock()

		if !c.blobs.Has(blobID) {
			debug.Log("tree %v references blob %v which isn't contained in index", id.Str(), blobID.Str())

			errs = append(errs, Error{TreeID: id, BlobID: blobID, Err: errors.New("not found in index")})
		}
	}

	return errs
}

// UnusedBlobs returns all blobs that have never been referenced.
func (c *Checker) UnusedBlobs() (blobs restic.IDs) {
	c.blobRefs.Lock()
	defer c.blobRefs.Unlock()

	debug.Log("checking %d blobs", len(c.blobs))
	for id := range c.blobs {
		if c.blobRefs.M[id] == 0 {
			debug.Log("blob %v not referenced", id.Str())
			blobs = append(blobs, id)
		}
	}

	return blobs
}

// CountPacks returns the number of packs in the repository.
func (c *Checker) CountPacks() uint64 {
	return uint64(len(c.packs))
}

// checkPack reads a pack and checks the integrity of all blobs.
func checkPack(r restic.Repository, id restic.ID) error {
	debug.Log("checking pack %v", id.Str())
	h := restic.Handle{Type: restic.DataFile, Name: id.String()}
	buf, err := backend.LoadAll(r.Backend(), h, nil)
	if err != nil {
		return err
	}

	hash := restic.Hash(buf)
	if !hash.Equal(id) {
		debug.Log("Pack ID does not match, want %v, got %v", id.Str(), hash.Str())
		return errors.Errorf("Pack ID does not match, want %v, got %v", id.Str(), hash.Str())
	}

	blobs, err := pack.List(r.Key(), bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return err
	}

	var errs []error
	for i, blob := range blobs {
		debug.Log("  check blob %d: %v", i, blob.ID.Str())

		plainBuf := make([]byte, blob.Length)
		n, err := crypto.Decrypt(r.Key(), plainBuf, buf[blob.Offset:blob.Offset+blob.Length])
		if err != nil {
			debug.Log("  error decrypting blob %v: %v", blob.ID.Str(), err)
			errs = append(errs, errors.Errorf("blob %v: %v", i, err))
			continue
		}
		plainBuf = plainBuf[:n]

		hash := restic.Hash(plainBuf)
		if !hash.Equal(blob.ID) {
			debug.Log("  Blob ID does not match, want %v, got %v", blob.ID.Str(), hash.Str())
			errs = append(errs, errors.Errorf("Blob ID does not match, want %v, got %v", blob.ID.Str(), hash.Str()))
			continue
		}
	}

	if len(errs) > 0 {
		return errors.Errorf("pack %v contains %v errors: %v", id.Str(), len(errs), errs)
	}

	return nil
}

// ReadData loads all data from the repository and checks the integrity.
func (c *Checker) ReadData(p *restic.Progress, errChan chan<- error, done <-chan struct{}) {
	defer close(errChan)

	p.Start()
	defer p.Done()

	worker := func(wg *sync.WaitGroup, in <-chan restic.ID) {
		defer wg.Done()
		for {
			var id restic.ID
			var ok bool

			select {
			case <-done:
				return
			case id, ok = <-in:
				if !ok {
					return
				}
			}

			err := checkPack(c.repo, id)
			p.Report(restic.Stat{Blobs: 1})
			if err == nil {
				continue
			}

			select {
			case <-done:
				return
			case errChan <- err:
			}
		}
	}

	ch := c.repo.List(restic.DataFile, done)

	var wg sync.WaitGroup
	for i := 0; i < defaultParallelism; i++ {
		wg.Add(1)
		go worker(&wg, ch)
	}

	wg.Wait()
}
