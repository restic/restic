package repository_test

import (
	"context"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/restic/restic/internal/checker"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
	"golang.org/x/sync/errgroup"
)

func testPrune(t *testing.T, opts repository.PruneOptions, errOnUnused bool) {
	seed := time.Now().UnixNano()
	random := rand.New(rand.NewSource(seed))
	t.Logf("rand initialized with seed %d", seed)

	repo, _, be := repository.TestRepositoryWithVersion(t, 0)
	createRandomBlobs(t, random, repo, 4, 0.5, true)
	createRandomBlobs(t, random, repo, 5, 0.5, true)
	keep, _ := selectBlobs(t, random, repo, 0.5)

	var wg errgroup.Group
	repo.StartPackUploader(context.TODO(), &wg)
	// duplicate a few blobs to exercise those code paths
	for blob := range keep {
		buf, err := repo.LoadBlob(context.TODO(), blob.Type, blob.ID, nil)
		rtest.OK(t, err)
		_, _, _, err = repo.SaveBlob(context.TODO(), blob.Type, buf, blob.ID, true)
		rtest.OK(t, err)
	}
	rtest.OK(t, repo.Flush(context.TODO()))

	plan, err := repository.PlanPrune(context.TODO(), opts, repo, func(ctx context.Context, repo restic.Repository, usedBlobs restic.FindBlobSet) error {
		for blob := range keep {
			usedBlobs.Insert(blob)
		}
		return nil
	}, &progress.NoopPrinter{})
	rtest.OK(t, err)

	rtest.OK(t, plan.Execute(context.TODO(), &progress.NoopPrinter{}))

	repo = repository.TestOpenBackend(t, be)
	checker.TestCheckRepo(t, repo, true)

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
				RepackSmall:    true,
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

// TestPruneMaxUnusedDuplicate checks that MaxUnused correctly accounts for duplicates.
//
// Create a repository containing blobs a to d that are stored in packs as follows:
// - a, d
// - b, d
// - c, d
// All blobs should be kept during prune, but the duplicates should be gone afterwards.
// The special construction ensures that each pack contains a used, non-duplicate blob.
// This ensures that special cases that delete completely duplicate packs files do not
// apply.
func TestPruneMaxUnusedDuplicate(t *testing.T) {
	seed := time.Now().UnixNano()
	random := rand.New(rand.NewSource(seed))
	t.Logf("rand initialized with seed %d", seed)

	repo, _, _ := repository.TestRepositoryWithVersion(t, 0)
	// large blobs to prevent repacking due to too small packsize
	const blobSize = 1024 * 1024

	bufs := [][]byte{}
	for i := 0; i < 4; i++ {
		// use uniform length for simpler control via MaxUnusedBytes
		buf := make([]byte, blobSize)
		random.Read(buf)
		bufs = append(bufs, buf)
	}
	keep := restic.NewBlobSet()

	for _, blobs := range [][][]byte{
		{bufs[0], bufs[3]},
		{bufs[1], bufs[3]},
		{bufs[2], bufs[3]},
	} {
		var wg errgroup.Group
		repo.StartPackUploader(context.TODO(), &wg)

		for _, blob := range blobs {
			id, _, _, err := repo.SaveBlob(context.TODO(), restic.DataBlob, blob, restic.ID{}, true)
			keep.Insert(restic.BlobHandle{Type: restic.DataBlob, ID: id})
			rtest.OK(t, err)
		}

		rtest.OK(t, repo.Flush(context.Background()))
	}

	opts := repository.PruneOptions{
		MaxRepackBytes: math.MaxUint64,
		// non-zero number of unused bytes, that is nevertheless smaller than a single blob
		// setting this to zero would bypass the unused/duplicate size accounting that should
		// be tested here
		MaxUnusedBytes: func(used uint64) (unused uint64) { return blobSize / 2 },
	}

	plan, err := repository.PlanPrune(context.TODO(), opts, repo, func(ctx context.Context, repo restic.Repository, usedBlobs restic.FindBlobSet) error {
		for blob := range keep {
			usedBlobs.Insert(blob)
		}
		return nil
	}, &progress.NoopPrinter{})
	rtest.OK(t, err)

	rtest.OK(t, plan.Execute(context.TODO(), &progress.NoopPrinter{}))

	rsize := plan.Stats().Size
	remainingUnusedSize := rsize.Duplicate + rsize.Unused - rsize.Remove - rsize.Repackrm
	maxUnusedSize := opts.MaxUnusedBytes(rsize.Used)
	rtest.Assert(t, remainingUnusedSize <= maxUnusedSize, "too much unused data remains got %v, expected less than %v", remainingUnusedSize, maxUnusedSize)

	// divide by blobSize to ignore pack file overhead
	rtest.Equals(t, rsize.Used/blobSize, uint64(4))
	rtest.Equals(t, rsize.Duplicate/blobSize, uint64(2))
	rtest.Equals(t, rsize.Unused, uint64(0))
	rtest.Equals(t, rsize.Remove, uint64(0))
	rtest.Equals(t, rsize.Repack/blobSize, uint64(4))
	rtest.Equals(t, rsize.Repackrm/blobSize, uint64(2))
	rtest.Equals(t, rsize.Unref, uint64(0))
	rtest.Equals(t, rsize.Uncompressed, uint64(0))
}
