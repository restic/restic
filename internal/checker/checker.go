package checker

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime"
	"sort"
	"sync"

	"github.com/minio/sha256-simd"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/s3"
	"github.com/restic/restic/internal/cache"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/hashing"
	"github.com/restic/restic/internal/index"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
	"golang.org/x/sync/errgroup"
)

// Checker runs various checks on a repository. It is advisable to create an
// exclusive Lock in the repository before running any checks.
//
// A Checker only tests for internal errors within the data structures of the
// repository (e.g. missing blobs), and needs a valid Repository to work on.
type Checker struct {
	packs    map[restic.ID]int64
	blobRefs struct {
		sync.Mutex
		M restic.BlobSet
	}
	trackUnused bool

	masterIndex *index.MasterIndex
	snapshots   restic.Lister

	repo restic.Repository
}

// New returns a new checker which runs on repo.
func New(repo restic.Repository, trackUnused bool) *Checker {
	c := &Checker{
		packs:       make(map[restic.ID]int64),
		masterIndex: index.NewMasterIndex(),
		repo:        repo,
		trackUnused: trackUnused,
	}

	c.blobRefs.M = restic.NewBlobSet()

	return c
}

// ErrLegacyLayout is returned when the repository uses the S3 legacy layout.
var ErrLegacyLayout = errors.New("repository uses S3 legacy layout")

// ErrDuplicatePacks is returned when a pack is found in more than one index.
type ErrDuplicatePacks struct {
	PackID  restic.ID
	Indexes restic.IDSet
}

func (e *ErrDuplicatePacks) Error() string {
	return fmt.Sprintf("pack %v contained in several indexes: %v", e.PackID, e.Indexes)
}

// ErrMixedPack is returned when a pack is found that contains both tree and data blobs.
type ErrMixedPack struct {
	PackID restic.ID
}

func (e *ErrMixedPack) Error() string {
	return fmt.Sprintf("pack %v contains a mix of tree and data blobs", e.PackID.Str())
}

// ErrOldIndexFormat is returned when an index with the old format is
// found.
type ErrOldIndexFormat struct {
	restic.ID
}

func (err *ErrOldIndexFormat) Error() string {
	return fmt.Sprintf("index %v has old format", err.ID)
}

func (c *Checker) LoadSnapshots(ctx context.Context) error {
	var err error
	c.snapshots, err = backend.MemorizeList(ctx, c.repo.Backend(), restic.SnapshotFile)
	return err
}

func computePackTypes(ctx context.Context, idx restic.MasterIndex) map[restic.ID]restic.BlobType {
	packs := make(map[restic.ID]restic.BlobType)
	idx.Each(ctx, func(pb restic.PackedBlob) {
		tpe, exists := packs[pb.PackID]
		if exists {
			if pb.Type != tpe {
				tpe = restic.InvalidBlob
			}
		} else {
			tpe = pb.Type
		}
		packs[pb.PackID] = tpe
	})
	return packs
}

// LoadIndex loads all index files.
func (c *Checker) LoadIndex(ctx context.Context) (hints []error, errs []error) {
	debug.Log("Start")

	packToIndex := make(map[restic.ID]restic.IDSet)
	err := index.ForAllIndexes(ctx, c.repo, func(id restic.ID, index *index.Index, oldFormat bool, err error) error {
		debug.Log("process index %v, err %v", id, err)

		if oldFormat {
			debug.Log("index %v has old format", id)
			hints = append(hints, &ErrOldIndexFormat{id})
		}

		err = errors.Wrapf(err, "error loading index %v", id)

		if err != nil {
			errs = append(errs, err)
			return nil
		}

		c.masterIndex.Insert(index)

		debug.Log("process blobs")
		cnt := 0
		index.Each(ctx, func(blob restic.PackedBlob) {
			cnt++

			if _, ok := packToIndex[blob.PackID]; !ok {
				packToIndex[blob.PackID] = restic.NewIDSet()
			}
			packToIndex[blob.PackID].Insert(id)
		})

		debug.Log("%d blobs processed", cnt)
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}

	// Merge index before computing pack sizes, as this needs removed duplicates
	err = c.masterIndex.MergeFinalIndexes()
	if err != nil {
		// abort if an error occurs merging the indexes
		return hints, append(errs, err)
	}

	// compute pack size using index entries
	c.packs = pack.Size(ctx, c.masterIndex, false)
	packTypes := computePackTypes(ctx, c.masterIndex)

	debug.Log("checking for duplicate packs")
	for packID := range c.packs {
		debug.Log("  check pack %v: contained in %d indexes", packID, len(packToIndex[packID]))
		if len(packToIndex[packID]) > 1 {
			hints = append(hints, &ErrDuplicatePacks{
				PackID:  packID,
				Indexes: packToIndex[packID],
			})
		}
		if packTypes[packID] == restic.InvalidBlob {
			hints = append(hints, &ErrMixedPack{
				PackID: packID,
			})
		}
	}

	err = c.repo.SetIndex(c.masterIndex)
	if err != nil {
		debug.Log("SetIndex returned error: %v", err)
		errs = append(errs, err)
	}

	return hints, errs
}

// PackError describes an error with a specific pack.
type PackError struct {
	ID       restic.ID
	Orphaned bool
	Err      error
}

func (e *PackError) Error() string {
	return "pack " + e.ID.String() + ": " + e.Err.Error()
}

// IsOrphanedPack returns true if the error describes a pack which is not
// contained in any index.
func IsOrphanedPack(err error) bool {
	var e *PackError
	return errors.As(err, &e) && e.Orphaned
}

func isS3Legacy(b restic.Backend) bool {
	// unwrap cache
	if be, ok := b.(*cache.Backend); ok {
		b = be.Backend
	}

	be, ok := b.(*s3.Backend)
	if !ok {
		return false
	}

	return be.Layout.Name() == "s3legacy"
}

// Packs checks that all packs referenced in the index are still available and
// there are no packs that aren't in an index. errChan is closed after all
// packs have been checked.
func (c *Checker) Packs(ctx context.Context, errChan chan<- error) {
	defer close(errChan)

	if isS3Legacy(c.repo.Backend()) {
		errChan <- ErrLegacyLayout
	}

	debug.Log("checking for %d packs", len(c.packs))

	debug.Log("listing repository packs")
	repoPacks := make(map[restic.ID]int64)

	err := c.repo.List(ctx, restic.PackFile, func(id restic.ID, size int64) error {
		repoPacks[id] = size
		return nil
	})

	if err != nil {
		errChan <- err
	}

	for id, size := range c.packs {
		reposize, ok := repoPacks[id]
		// remove from repoPacks so we can find orphaned packs
		delete(repoPacks, id)

		// missing: present in c.packs but not in the repo
		if !ok {
			select {
			case <-ctx.Done():
				return
			case errChan <- &PackError{ID: id, Err: errors.New("does not exist")}:
			}
			continue
		}

		// size not matching: present in c.packs and in the repo, but sizes do not match
		if size != reposize {
			select {
			case <-ctx.Done():
				return
			case errChan <- &PackError{ID: id, Err: errors.Errorf("unexpected file size: got %d, expected %d", reposize, size)}:
			}
		}
	}

	// orphaned: present in the repo but not in c.packs
	for orphanID := range repoPacks {
		select {
		case <-ctx.Done():
			return
		case errChan <- &PackError{ID: orphanID, Orphaned: true, Err: errors.New("not referenced in any index")}:
		}
	}
}

// Error is an error that occurred while checking a repository.
type Error struct {
	TreeID restic.ID
	Err    error
}

func (e *Error) Error() string {
	if !e.TreeID.IsNull() {
		return "tree " + e.TreeID.String() + ": " + e.Err.Error()
	}

	return e.Err.Error()
}

// TreeError collects several errors that occurred while processing a tree.
type TreeError struct {
	ID     restic.ID
	Errors []error
}

func (e *TreeError) Error() string {
	return fmt.Sprintf("tree %v: %v", e.ID, e.Errors)
}

// checkTreeWorker checks the trees received and sends out errors to errChan.
func (c *Checker) checkTreeWorker(ctx context.Context, trees <-chan restic.TreeItem, out chan<- error) {
	for job := range trees {
		debug.Log("check tree %v (tree %v, err %v)", job.ID, job.Tree, job.Error)

		var errs []error
		if job.Error != nil {
			errs = append(errs, job.Error)
		} else {
			errs = c.checkTree(job.ID, job.Tree)
		}

		if len(errs) == 0 {
			continue
		}
		treeError := &TreeError{ID: job.ID, Errors: errs}
		select {
		case <-ctx.Done():
			return
		case out <- treeError:
			debug.Log("tree %v: sent %d errors", treeError.ID, len(treeError.Errors))
		}
	}
}

func loadSnapshotTreeIDs(ctx context.Context, lister restic.Lister, repo restic.Repository) (ids restic.IDs, errs []error) {
	err := restic.ForAllSnapshots(ctx, lister, repo, nil, func(id restic.ID, sn *restic.Snapshot, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		treeID := *sn.Tree
		debug.Log("snapshot %v has tree %v", id, treeID)
		ids = append(ids, treeID)
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}

	return ids, errs
}

// Structure checks that for all snapshots all referenced data blobs and
// subtrees are available in the index. errChan is closed after all trees have
// been traversed.
func (c *Checker) Structure(ctx context.Context, p *progress.Counter, errChan chan<- error) {
	trees, errs := loadSnapshotTreeIDs(ctx, c.snapshots, c.repo)
	p.SetMax(uint64(len(trees)))
	debug.Log("need to check %d trees from snapshots, %d errs returned", len(trees), len(errs))

	for _, err := range errs {
		select {
		case <-ctx.Done():
			return
		case errChan <- err:
		}
	}

	wg, ctx := errgroup.WithContext(ctx)
	treeStream := restic.StreamTrees(ctx, wg, c.repo, trees, func(treeID restic.ID) bool {
		// blobRefs may be accessed in parallel by checkTree
		c.blobRefs.Lock()
		h := restic.BlobHandle{ID: treeID, Type: restic.TreeBlob}
		blobReferenced := c.blobRefs.M.Has(h)
		// noop if already referenced
		c.blobRefs.M.Insert(h)
		c.blobRefs.Unlock()
		return blobReferenced
	}, p)

	defer close(errChan)
	// The checkTree worker only processes already decoded trees and is thus CPU-bound
	workerCount := runtime.GOMAXPROCS(0)
	for i := 0; i < workerCount; i++ {
		wg.Go(func() error {
			c.checkTreeWorker(ctx, treeStream, errChan)
			return nil
		})
	}

	// the wait group should not return an error because no worker returns an
	// error, so panic if that has changed somehow.
	err := wg.Wait()
	if err != nil {
		panic(err)
	}
}

func (c *Checker) checkTree(id restic.ID, tree *restic.Tree) (errs []error) {
	debug.Log("checking tree %v", id)

	for _, node := range tree.Nodes {
		switch node.Type {
		case "file":
			if node.Content == nil {
				errs = append(errs, &Error{TreeID: id, Err: errors.Errorf("file %q has nil blob list", node.Name)})
			}

			for b, blobID := range node.Content {
				if blobID.IsNull() {
					errs = append(errs, &Error{TreeID: id, Err: errors.Errorf("file %q blob %d has null ID", node.Name, b)})
					continue
				}
				// Note that we do not use the blob size. The "obvious" check
				// whether the sum of the blob sizes matches the file size
				// unfortunately fails in some cases that are not resolveable
				// by users, so we omit this check, see #1887

				_, found := c.repo.LookupBlobSize(blobID, restic.DataBlob)
				if !found {
					debug.Log("tree %v references blob %v which isn't contained in index", id, blobID)
					errs = append(errs, &Error{TreeID: id, Err: errors.Errorf("file %q blob %v not found in index", node.Name, blobID)})
				}
			}

			if c.trackUnused {
				// loop a second time to keep the locked section as short as possible
				c.blobRefs.Lock()
				for _, blobID := range node.Content {
					if blobID.IsNull() {
						continue
					}
					h := restic.BlobHandle{ID: blobID, Type: restic.DataBlob}
					c.blobRefs.M.Insert(h)
					debug.Log("blob %v is referenced", blobID)
				}
				c.blobRefs.Unlock()
			}

		case "dir":
			if node.Subtree == nil {
				errs = append(errs, &Error{TreeID: id, Err: errors.Errorf("dir node %q has no subtree", node.Name)})
				continue
			}

			if node.Subtree.IsNull() {
				errs = append(errs, &Error{TreeID: id, Err: errors.Errorf("dir node %q subtree id is null", node.Name)})
				continue
			}

		case "symlink", "socket", "chardev", "dev", "fifo":
			// nothing to check

		default:
			errs = append(errs, &Error{TreeID: id, Err: errors.Errorf("node %q with invalid type %q", node.Name, node.Type)})
		}

		if node.Name == "" {
			errs = append(errs, &Error{TreeID: id, Err: errors.New("node with empty name")})
		}
	}

	return errs
}

// UnusedBlobs returns all blobs that have never been referenced.
func (c *Checker) UnusedBlobs(ctx context.Context) (blobs restic.BlobHandles) {
	if !c.trackUnused {
		panic("only works when tracking blob references")
	}
	c.blobRefs.Lock()
	defer c.blobRefs.Unlock()

	debug.Log("checking %d blobs", len(c.blobRefs.M))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	c.repo.Index().Each(ctx, func(blob restic.PackedBlob) {
		h := restic.BlobHandle{ID: blob.ID, Type: blob.Type}
		if !c.blobRefs.M.Has(h) {
			debug.Log("blob %v not referenced", h)
			blobs = append(blobs, h)
		}
	})

	return blobs
}

// CountPacks returns the number of packs in the repository.
func (c *Checker) CountPacks() uint64 {
	return uint64(len(c.packs))
}

// GetPacks returns IDSet of packs in the repository
func (c *Checker) GetPacks() map[restic.ID]int64 {
	return c.packs
}

// checkPack reads a pack and checks the integrity of all blobs.
func checkPack(ctx context.Context, r restic.Repository, id restic.ID, blobs []restic.Blob, size int64, bufRd *bufio.Reader) error {
	debug.Log("checking pack %v", id.String())

	if len(blobs) == 0 {
		return errors.Errorf("pack %v is empty or not indexed", id)
	}

	// sanity check blobs in index
	sort.Slice(blobs, func(i, j int) bool {
		return blobs[i].Offset < blobs[j].Offset
	})
	idxHdrSize := pack.CalculateHeaderSize(blobs)
	lastBlobEnd := 0
	nonContinuousPack := false
	for _, blob := range blobs {
		if lastBlobEnd != int(blob.Offset) {
			nonContinuousPack = true
		}
		lastBlobEnd = int(blob.Offset + blob.Length)
	}
	// size was calculated by masterindex.PackSize, thus there's no need to recalculate it here

	var errs []error
	if nonContinuousPack {
		debug.Log("Index for pack contains gaps / overlaps, blobs: %v", blobs)
		errs = append(errs, errors.New("Index for pack contains gaps / overlapping blobs"))
	}

	// calculate hash on-the-fly while reading the pack and capture pack header
	var hash restic.ID
	var hdrBuf []byte
	hashingLoader := func(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
		return r.Backend().Load(ctx, h, int(size), 0, func(rd io.Reader) error {
			hrd := hashing.NewReader(rd, sha256.New())
			bufRd.Reset(hrd)

			// skip to start of first blob, offset == 0 for correct pack files
			_, err := bufRd.Discard(int(offset))
			if err != nil {
				return err
			}

			err = fn(bufRd)
			if err != nil {
				return err
			}

			// skip enough bytes until we reach the possible header start
			curPos := length + int(offset)
			minHdrStart := int(size) - pack.MaxHeaderSize
			if minHdrStart > curPos {
				_, err := bufRd.Discard(minHdrStart - curPos)
				if err != nil {
					return err
				}
			}

			// read remainder, which should be the pack header
			hdrBuf, err = io.ReadAll(bufRd)
			if err != nil {
				return err
			}

			hash = restic.IDFromHash(hrd.Sum(nil))
			return nil
		})
	}

	err := repository.StreamPack(ctx, hashingLoader, r.Key(), id, blobs, func(blob restic.BlobHandle, buf []byte, err error) error {
		debug.Log("  check blob %v: %v", blob.ID, blob)
		if err != nil {
			debug.Log("  error verifying blob %v: %v", blob.ID, err)
			errs = append(errs, errors.Errorf("blob %v: %v", blob.ID, err))
		}
		return nil
	})
	if err != nil {
		// failed to load the pack file, return as further checks cannot succeed anyways
		debug.Log("  error streaming pack: %v", err)
		return errors.Errorf("pack %v failed to download: %v", id, err)
	}
	if !hash.Equal(id) {
		debug.Log("Pack ID does not match, want %v, got %v", id, hash)
		return errors.Errorf("Pack ID does not match, want %v, got %v", id, hash)
	}

	blobs, hdrSize, err := pack.List(r.Key(), bytes.NewReader(hdrBuf), int64(len(hdrBuf)))
	if err != nil {
		return err
	}

	if uint32(idxHdrSize) != hdrSize {
		debug.Log("Pack header size does not match, want %v, got %v", idxHdrSize, hdrSize)
		errs = append(errs, errors.Errorf("Pack header size does not match, want %v, got %v", idxHdrSize, hdrSize))
	}

	idx := r.Index()
	for _, blob := range blobs {
		// Check if blob is contained in index and position is correct
		idxHas := false
		for _, pb := range idx.Lookup(blob.BlobHandle) {
			if pb.PackID == id && pb.Blob == blob {
				idxHas = true
				break
			}
		}
		if !idxHas {
			errs = append(errs, errors.Errorf("Blob %v is not contained in index or position is incorrect", blob.ID))
			continue
		}
	}

	if len(errs) > 0 {
		return errors.Errorf("pack %v contains %v errors: %v", id, len(errs), errs)
	}

	return nil
}

// ReadData loads all data from the repository and checks the integrity.
func (c *Checker) ReadData(ctx context.Context, errChan chan<- error) {
	c.ReadPacks(ctx, c.packs, nil, errChan)
}

// ReadPacks loads data from specified packs and checks the integrity.
func (c *Checker) ReadPacks(ctx context.Context, packs map[restic.ID]int64, p *progress.Counter, errChan chan<- error) {
	defer close(errChan)

	g, ctx := errgroup.WithContext(ctx)
	type checkTask struct {
		id    restic.ID
		size  int64
		blobs []restic.Blob
	}
	ch := make(chan checkTask)

	// as packs are streamed the concurrency is limited by IO
	workerCount := int(c.repo.Connections())
	// run workers
	for i := 0; i < workerCount; i++ {
		g.Go(func() error {
			// create a buffer that is large enough to be reused by repository.StreamPack
			// this ensures that we can read the pack header later on
			bufRd := bufio.NewReaderSize(nil, repository.MaxStreamBufferSize)
			for {
				var ps checkTask
				var ok bool

				select {
				case <-ctx.Done():
					return nil
				case ps, ok = <-ch:
					if !ok {
						return nil
					}
				}

				err := checkPack(ctx, c.repo, ps.id, ps.blobs, ps.size, bufRd)
				p.Add(1)
				if err == nil {
					continue
				}

				select {
				case <-ctx.Done():
					return nil
				case errChan <- err:
				}
			}
		})
	}

	packSet := restic.NewIDSet()
	for pack := range packs {
		packSet.Insert(pack)
	}

	// push packs to ch
	for pbs := range c.repo.Index().ListPacks(ctx, packSet) {
		size := packs[pbs.PackID]
		debug.Log("listed %v", pbs.PackID)
		select {
		case ch <- checkTask{id: pbs.PackID, size: size, blobs: pbs.Blobs}:
		case <-ctx.Done():
		}
	}
	close(ch)

	err := g.Wait()
	if err != nil {
		select {
		case <-ctx.Done():
			return
		case errChan <- err:
		}
	}
}
