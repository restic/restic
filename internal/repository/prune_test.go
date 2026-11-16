package repository_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	resticerrors "github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func testPrune(t *testing.T, opts repository.PruneOptions, errOnUnused bool) {
	seed := time.Now().UnixNano()
	random := rand.New(rand.NewSource(seed))
	t.Logf("rand initialized with seed %d", seed)

	repo, _, be := repository.TestRepositoryWithVersion(t, 0)
	createRandomBlobs(t, random, repo, 4, 0.5, true)
	createRandomBlobs(t, random, repo, 5, 0.5, true)
	keep, _ := selectBlobs(t, random, repo, 0.5)

	rtest.OK(t, repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		// duplicate a few blobs to exercise those code paths
		for blob := range keep {
			buf, err := repo.LoadBlob(ctx, blob, nil)
			rtest.OK(t, err)
			_, _, _, err = uploader.SaveBlob(ctx, blob.Type, buf, blob.ID, true)
			rtest.OK(t, err)
		}
		return nil
	}))

	plan, err := repository.PlanPrune(context.TODO(), opts, repo, func(ctx context.Context, repo restic.Repository, usedBlobs restic.FindBlobSet) error {
		for blob := range keep {
			usedBlobs.Insert(blob)
		}
		return nil
	}, restic.NewNoopPrinter())
	rtest.OK(t, err)

	rtest.OK(t, plan.Execute(context.TODO(), restic.NewNoopPrinter()))

	repo = repository.TestOpenBackend(t, be)
	repository.TestCheckRepo(t, repo)

	if errOnUnused {
		existing := listBlobs(repo)
		rtest.Assert(t, existing.Equals(keep), "unexpected blobs, wanted %v got %v", keep, existing)
	}
}

func TestPrune(t *testing.T) {
	for _, test := range []struct {
		name        string
		opts        repository.PruneOptions
		errOnUnused bool
	}{
		{
			name: "0",
			opts: repository.PruneOptions{
				MaxRepackBytes: math.MaxUint64,
				MaxUnusedBytes: func(used uint64) (unused uint64) { return 0 },
			},
			errOnUnused: true,
		},
		{
			name: "50",
			opts: repository.PruneOptions{
				MaxRepackBytes: math.MaxUint64,
				MaxUnusedBytes: func(used uint64) (unused uint64) { return used / 2 },
			},
		},
		{
			name: "unlimited",
			opts: repository.PruneOptions{
				MaxRepackBytes: math.MaxUint64,
				MaxUnusedBytes: func(used uint64) (unused uint64) { return math.MaxUint64 },
			},
		},
		{
			name: "cachableonly",
			opts: repository.PruneOptions{
				MaxRepackBytes:      math.MaxUint64,
				MaxUnusedBytes:      func(used uint64) (unused uint64) { return used / 20 },
				RepackCacheableOnly: true,
			},
		},
		{
			name: "small",
			opts: repository.PruneOptions{
				MaxRepackBytes: math.MaxUint64,
				MaxUnusedBytes: func(used uint64) (unused uint64) { return math.MaxUint64 },
			},
			errOnUnused: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			testPrune(t, test.opts, test.errOnUnused)
		})
		t.Run(test.name+"-recovery", func(t *testing.T) {
			opts := test.opts
			opts.UnsafeRecovery = true
			// unsafeNoSpaceRecovery does not repack partially used pack files
			testPrune(t, opts, false)
		})
	}
}

/*
1.) create repository with packsize of 2M.
2.) create enough data for 11 packfiles (31 packs)
3.) run a repository.PlanPrune(...) with a packsize of 16M (current default).
4.) run plan.Execute(...), extract plan.Stats() and check.
5.) Check that all blobs are contained in the new packfiles.
6.) The result should be less packfiles than before
*/
func TestPruneSmall(t *testing.T) {
	t.Parallel()
	seed := time.Now().UnixNano()
	random := rand.New(rand.NewSource(seed))
	t.Logf("rand initialized with seed %d", seed)

	be := repository.TestBackend(t)
	repo, _ := repository.TestRepositoryWithBackend(t, be, 0, repository.Options{PackSize: repository.MinPackSize, Compression: repository.CompressionOff})

	const blobSize = 1000 * 1000
	const numBlobsCreated = 55

	keep := restic.NewBlobSet()
	rtest.OK(t, repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		// we need a minimum of 11 packfiles, each packfile will be about 5 Mb long
		for range numBlobsCreated {
			buf := make([]byte, blobSize)
			random.Read(buf)

			id, _, _, err := uploader.SaveBlob(ctx, restic.DataBlob, buf, restic.ID{}, false)
			rtest.OK(t, err)
			keep.Insert(restic.BlobHandle{Type: restic.DataBlob, ID: id})
		}
		return nil
	}))

	// gather number of packfiles
	repoPacks, err := pack.Size(context.TODO(), repo, false)
	rtest.OK(t, err)
	lenPackfilesBefore := len(repoPacks)
	rtest.OK(t, repo.Close())

	// and reopen repository with default packsize
	repo = repository.TestOpenBackend(t, be)
	rtest.OK(t, repo.LoadIndex(context.TODO(), restic.NoopTerminalCounterFactory))

	opts := repository.PruneOptions{
		MaxRepackBytes: math.MaxUint64,
		MaxUnusedBytes: func(used uint64) (unused uint64) { return blobSize / 4 },
		SmallPackBytes: 5 * 1024 * 1024,
	}
	plan, err := repository.PlanPrune(context.TODO(), opts, repo, func(ctx context.Context, repo restic.Repository, usedBlobs restic.FindBlobSet) error {
		for blob := range keep {
			usedBlobs.Insert(blob)
		}
		return nil
	}, restic.NewNoopPrinter())
	rtest.OK(t, err)
	rtest.OK(t, plan.Execute(context.TODO(), restic.NewNoopPrinter()))

	stats := plan.Stats()
	rtest.Equals(t, stats.Size.Used/blobSize, uint64(numBlobsCreated), fmt.Sprintf("total size of blobs should be %d but is %d",
		numBlobsCreated, stats.Size.Used/blobSize))
	rtest.Equals(t, stats.Blobs.Used, stats.Blobs.Repack, "the number of blobs should be identical after a repack")

	// repopen repository
	repo = repository.TestOpenBackend(t, be)
	repository.TestCheckRepo(t, repo)

	// load all blobs
	for blob := range keep {
		_, err := repo.LoadBlob(context.TODO(), blob, nil)
		rtest.OK(t, err)
	}

	repoPacks, err = pack.Size(context.TODO(), repo, false)
	rtest.OK(t, err)
	lenPackfilesAfter := len(repoPacks)

	rtest.Equals(t, lenPackfilesBefore > lenPackfilesAfter, true,
		fmt.Sprintf("the number packfiles before %d and after repack %d", lenPackfilesBefore, lenPackfilesAfter))
}

type failingListBackend struct {
	backend.Backend
	err error
}

func (b *failingListBackend) List(ctx context.Context, t backend.FileType, fn func(backend.FileInfo) error) error {
	if t == backend.PackFile {
		return b.err
	}
	return b.Backend.List(ctx, t, fn)
}

func TestPrunePackListError(t *testing.T) {
	_, _, be := repository.TestRepositoryWithVersion(t, 0)

	listErr := errors.New("request timeout")
	repo := repository.TestOpenBackend(t, &failingListBackend{Backend: be, err: listErr})
	rtest.OK(t, repo.LoadIndex(context.TODO(), restic.NoopTerminalCounterFactory))

	_, err := repository.PlanPrune(context.TODO(), repository.PruneOptions{
		MaxRepackBytes: math.MaxUint64,
		MaxUnusedBytes: func(used uint64) uint64 { return used },
	}, repo, func(ctx context.Context, repo restic.Repository, usedBlobs restic.FindBlobSet) error {
		return nil
	}, restic.NewNoopPrinter())
	rtest.Assert(t, err != nil, "expected an error")
	rtest.Assert(t, resticerrors.IsFatal(err), "expected a fatal error, got %v", err)
	rtest.Assert(t, errors.Is(err, listErr), "expected error to wrap the list error, got %v", err)
	rtest.Assert(t, strings.Contains(err.Error(), "failed to list pack files"), "unexpected message %q", err)
}

// prepareRemoveBlobs creates a repository with used blobs and returns the used
// blobs and some unused blobs which share a pack file with a used blob.
func prepareRemoveBlobs(t *testing.T) (*repository.Repository, backend.Backend, restic.BlobSet, restic.BlobSet) {
	seed := time.Now().UnixNano()
	random := rand.New(rand.NewSource(seed))
	t.Logf("rand initialized with seed %d", seed)

	repo, _, be := repository.TestRepositoryWithVersion(t, 0)
	createRandomBlobs(t, random, repo, 20, 0.7, false)
	createRandomBlobs(t, random, repo, 20, 0.7, false)
	used, unused := selectBlobs(t, random, repo, 0.6)

	usedPacks := findPacksForBlobs(t, repo, used)
	remove := restic.NewBlobSet()
	for h := range unused {
		for _, pb := range repo.LookupBlob(h) {
			if usedPacks.Has(pb.PackID()) && random.Float32() < 0.5 {
				remove.Insert(h)
			}
		}
	}
	if len(remove) == 0 {
		t.Skip("no unused blob shares a pack file with a used blob")
	}
	return repo, be, used, remove
}

func planRemoveBlobs(t *testing.T, repo *repository.Repository, used, remove restic.BlobSet) *repository.PrunePlan {
	// tolerate any amount of unused data, so that only RemoveBlobs causes repacking
	opts := repository.PruneOptions{
		MaxRepackBytes: math.MaxUint64,
		MaxUnusedBytes: func(used uint64) uint64 { return math.MaxUint64 },
		RemoveBlobs:    remove,
	}
	plan, err := repository.PlanPrune(context.TODO(), opts, repo, func(ctx context.Context, repo restic.Repository, usedBlobs restic.FindBlobSet) error {
		for blob := range used {
			usedBlobs.Insert(blob)
		}
		return nil
	}, restic.NewNoopPrinter())
	rtest.OK(t, err)
	return plan
}

func TestPruneRemoveBlobs(t *testing.T) {
	repo, be, used, remove := prepareRemoveBlobs(t)

	// a used blob in RemoveBlobs is kept
	var usedTarget restic.BlobHandle
	for h := range used {
		usedTarget = h
		break
	}
	targets := restic.NewBlobSet(usedTarget)
	targets.Merge(remove)

	plan := planRemoveBlobs(t, repo, used, targets)
	rtest.Assert(t, plan.Stats().Packs.Repack > 0, "no pack file selected for repacking")
	rtest.OK(t, plan.Execute(context.TODO(), restic.NewNoopPrinter()))

	repo = repository.TestOpenBackend(t, be)
	repository.TestCheckRepo(t, repo)
	rtest.OK(t, repo.LoadIndex(context.TODO(), restic.NoopTerminalCounterFactory))

	existing := listBlobs(repo)
	for h := range remove {
		rtest.Assert(t, !existing.Has(h), "blob %v was not removed", h)
	}
	for h := range used {
		_, err := repo.LoadBlob(context.TODO(), h, nil)
		rtest.OK(t, err)
	}
}

type failingRemoveBackend struct {
	backend.Backend
}

func (b *failingRemoveBackend) Remove(ctx context.Context, h backend.Handle) error {
	if h.Type == backend.PackFile {
		return errors.New("remove failed")
	}
	return b.Backend.Remove(ctx, h)
}

func TestPruneRemoveBlobsDeleteError(t *testing.T) {
	repo, be, used, remove := prepareRemoveBlobs(t)
	rtest.OK(t, repo.Close())

	repo = repository.TestOpenBackend(t, &failingRemoveBackend{Backend: be})
	rtest.OK(t, repo.LoadIndex(context.TODO(), restic.NoopTerminalCounterFactory))
	plan := planRemoveBlobs(t, repo, used, remove)
	err := plan.Execute(context.TODO(), restic.NewNoopPrinter())
	rtest.Assert(t, err != nil, "a pack file which could not be removed was not reported")
}
