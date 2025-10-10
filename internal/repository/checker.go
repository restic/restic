package repository

import (
	"bufio"
	"context"
	"fmt"

	"github.com/klauspost/compress/zstd"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository/index"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
	"golang.org/x/sync/errgroup"
)

const maxStreamBufferSize = 4 * 1024 * 1024

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

// PackError describes an error with a specific pack.
type PackError struct {
	ID        restic.ID
	Orphaned  bool
	Truncated bool
	Err       error
}

func (e *PackError) Error() string {
	return "pack " + e.ID.String() + ": " + e.Err.Error()
}

// Checker handles index-related operations for repository checking.
type Checker struct {
	repo *Repository
}

// NewChecker creates a new Checker.
func NewChecker(repo *Repository) *Checker {
	return &Checker{
		repo: repo,
	}
}
func computePackTypes(ctx context.Context, idx restic.ListBlobser) (map[restic.ID]restic.BlobType, error) {
	packs := make(map[restic.ID]restic.BlobType)
	err := idx.ListBlobs(ctx, func(pb restic.PackedBlob) {
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
	return packs, err
}

// LoadIndex loads all index files.
func (c *Checker) LoadIndex(ctx context.Context, p restic.TerminalCounterFactory) (hints []error, errs []error) {
	debug.Log("Start")
	packToIndex := make(map[restic.ID]restic.IDSet)

	// Use the repository's internal loadIndexWithCallback to handle per-index errors
	err := c.repo.loadIndexWithCallback(ctx, p, func(id restic.ID, idx *index.Index, err error) error {
		debug.Log("process index %v, err %v", id, err)
		err = errors.Wrapf(err, "error loading index %v", id)

		if err != nil {
			errs = append(errs, err)
			return nil
		}

		debug.Log("process blobs")
		cnt := 0
		for blob := range idx.Values() {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			cnt++

			if _, ok := packToIndex[blob.PackID]; !ok {
				packToIndex[blob.PackID] = restic.NewIDSet()
			}
			packToIndex[blob.PackID].Insert(id)
		}

		debug.Log("%d blobs processed", cnt)
		return nil
	})
	if err != nil {
		// failed to load the index
		return hints, append(errs, err)
	}

	packTypes, err := computePackTypes(ctx, c.repo)
	if err != nil {
		return hints, append(errs, err)
	}

	debug.Log("checking for duplicate packs")
	for packID := range packTypes {
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

	return hints, errs
}

// Packs checks that all packs referenced in the index are still available and
// there are no packs that aren't in an index. errChan is closed after all
// packs have been checked.
func (c *Checker) Packs(ctx context.Context, errChan chan<- error) {
	defer close(errChan)

	// compute pack size using index entries
	packs, err := pack.Size(ctx, c.repo, false)
	if err != nil {
		errChan <- err
		return
	}

	debug.Log("checking for %d packs", len(packs))

	debug.Log("listing repository packs")
	repoPacks := make(map[restic.ID]int64)

	err = c.repo.List(ctx, restic.PackFile, func(id restic.ID, size int64) error {
		repoPacks[id] = size
		return nil
	})

	if err != nil {
		errChan <- err
	}

	for id, size := range packs {
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
			case errChan <- &PackError{ID: id, Truncated: true, Err: errors.Errorf("unexpected file size: got %d, expected %d", reposize, size)}:
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

// ReadPacks loads data from specified packs and checks the integrity.
func (c *Checker) ReadPacks(ctx context.Context, filter func(packs map[restic.ID]int64) map[restic.ID]int64, p *progress.Counter, errChan chan<- error) {
	defer close(errChan)

	// compute pack size using index entries
	packs, err := pack.Size(ctx, c.repo, false)
	if err != nil {
		errChan <- err
		return
	}
	packs = filter(packs)
	p.SetMax(uint64(len(packs)))

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
			bufRd := bufio.NewReaderSize(nil, maxStreamBufferSize)
			dec, err := zstd.NewReader(nil)
			if err != nil {
				panic(dec)
			}
			defer dec.Close()
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

				err := CheckPack(ctx, c.repo, ps.id, ps.blobs, ps.size, bufRd, dec)
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
	for pbs := range c.repo.ListPacksFromIndex(ctx, packSet) {
		size := packs[pbs.PackID]
		debug.Log("listed %v", pbs.PackID)
		select {
		case ch <- checkTask{id: pbs.PackID, size: size, blobs: pbs.Blobs}:
		case <-ctx.Done():
		}
	}
	close(ch)

	err = g.Wait()
	if err != nil {
		select {
		case <-ctx.Done():
			return
		case errChan <- err:
		}
	}
}
