package repository_test

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/restic/restic/internal/checker"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/repository/pack"
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
1.) create repository with packsize of 5M.
2.) create enough data for 11 packfiles (55 packs)
3.) run a repository.PlanPrune(...) with a packsize of 16M (current default).
4.) run plan.Execute(...), extract plan.Stats() and check.
5.) Check that all blobs are contained in the new packfiles.
6.) The result should be less packfiles than before
*/
func TestPruneSmall(t *testing.T) {
	seed := time.Now().UnixNano()
	random := rand.New(rand.NewSource(seed))

	be := repository.TestBackend(t)
	repo, _ := repository.TestRepositoryWithBackend(t, be, 0, repository.Options{PackSize: repository.MinPackSize})

	const blobSize = 1000 * 1000
	const numBlobsCreated = 55

	var wg errgroup.Group
	repo.StartPackUploader(context.TODO(), &wg)
	keep := restic.NewBlobSet()
	for i := 0; i < numBlobsCreated; i++ {
		buf := make([]byte, blobSize)
		random.Read(buf)

		id, _, _, err := repo.SaveBlob(context.TODO(), restic.DataBlob, buf, restic.ID{}, false)
		rtest.OK(t, err)
		keep.Insert(restic.BlobHandle{Type: restic.DataBlob, ID: id})
	}
	rtest.OK(t, repo.Flush(context.Background()))

	// gather number of packfiles
	repoPacks, err := pack.Size(context.TODO(), repo, false)
	rtest.OK(t, err)
	lenPackfilesBefore := len(repoPacks)
	rtest.OK(t, repo.Close())

	// and reopen repository with default packsize
	repo = repository.TestOpenBackend(t, be)
	rtest.OK(t, repo.LoadIndex(context.TODO(), nil))

	opts := repository.PruneOptions{
		MaxRepackBytes: math.MaxUint64,
		MaxUnusedBytes: func(used uint64) (unused uint64) { return 0 },
		SmallPackBytes: 5 * 1024 * 1024,
	}
	plan, err := repository.PlanPrune(context.TODO(), opts, repo, func(ctx context.Context, repo restic.Repository, usedBlobs restic.FindBlobSet) error {
		for blob := range keep {
			usedBlobs.Insert(blob)
		}
		return nil
	}, &progress.NoopPrinter{})
	rtest.OK(t, err)
	rtest.OK(t, plan.Execute(context.TODO(), &progress.NoopPrinter{}))

	stats := plan.Stats()
	rtest.Assert(t, stats.Size.Used/blobSize == uint64(numBlobsCreated), "total size of blobs should be %d but is %d",
		numBlobsCreated, stats.Size.Used/blobSize)

	// repopen repository
	repo = repository.TestOpenBackend(t, be)
	checker.TestCheckRepo(t, repo, true)

	// load all blobs
	for blob := range keep {
		_, err := repo.LoadBlob(context.TODO(), blob.Type, blob.ID, nil)
		rtest.OK(t, err)
	}

	repoPacks, err = pack.Size(context.TODO(), repo, false)
	rtest.OK(t, err)
	lenPackfilesAfter := len(repoPacks)

	rtest.Assert(t, lenPackfilesBefore > lenPackfilesAfter,
		"the number packfiles before %d and after repack %d", lenPackfilesBefore, lenPackfilesAfter)
}

func TestPrunePercentile(t *testing.T) {
	// pass the low limit test in prune.decidePackAction()
	const numPackfiles = 10
	random := rand.New(rand.NewSource(time.Now().UnixNano()))

	be := repository.TestBackend(t)
	repo, _ := repository.TestRepositoryWithBackend(t, be, 0, repository.Options{PackSize: repository.MinPackSize})

	var wg errgroup.Group
	keep := restic.NewBlobSet()
	// we create <numPackfiles> packfiles with 5 blobs each,  blob size is 1 kiB
	for packfiles := 0; packfiles < numPackfiles; packfiles++ {
		repo.StartPackUploader(context.TODO(), &wg)
		for i := 0; i < 5; i++ {
			buf := make([]byte, 512)
			random.Read(buf)

			id, _, _, err := repo.SaveBlob(context.TODO(), restic.DataBlob, buf, restic.ID{}, false)
			rtest.OK(t, err)
			keep.Insert(restic.BlobHandle{Type: restic.DataBlob, ID: id})
		}
		rtest.OK(t, repo.Flush(context.Background()))
	}

	// and 2 big one
	repo.StartPackUploader(context.TODO(), &wg)
	for i := 0; i < 2; i++ {
		buf := make([]byte, 1024*1024)
		random.Read(buf)

		id, _, _, err := repo.SaveBlob(context.TODO(), restic.DataBlob, buf, restic.ID{}, false)
		rtest.OK(t, err)
		keep.Insert(restic.BlobHandle{Type: restic.DataBlob, ID: id})
	}
	rtest.OK(t, repo.Flush(context.Background()))

	// gather number of packfiles
	repoPacks, err := pack.Size(context.TODO(), repo, false)
	rtest.OK(t, err)
	lenPackfilesBefore := len(repoPacks)
	rtest.OK(t, repo.LoadIndex(context.TODO(), nil))
	rtest.Assert(t, lenPackfilesBefore >= numPackfiles+1, "there should be at least %d packfiles, but there are %d",
		lenPackfilesBefore, numPackfiles+1)
	rtest.OK(t, repo.Close())

	// and reopen repository with default packsize
	repo = repository.TestOpenBackend(t, be)
	rtest.OK(t, repo.LoadIndex(context.TODO(), nil))

	// calculate blob sizes
	sizeCompressed := uint(0)
	err = repo.ListBlobs(context.TODO(), func(blob restic.PackedBlob) {
		sizeCompressed += blob.Length
	})
	rtest.OK(t, err)

	opts := repository.PruneOptions{
		MaxRepackBytes: math.MaxUint64,
		MaxUnusedBytes: func(used uint64) (unused uint64) { return 0 },
	}
	plan, err := repository.PlanPrune(context.TODO(), opts, repo, func(ctx context.Context, repo restic.Repository, usedBlobs restic.FindBlobSet) error {
		for blob := range keep {
			usedBlobs.Insert(blob)
		}
		return nil
	}, &progress.NoopPrinter{})
	rtest.OK(t, err)
	rtest.OK(t, plan.Execute(context.TODO(), &progress.NoopPrinter{}))

	stats := plan.Stats()
	rtest.Assert(t, stats.Blobs.Used == uint(len(keep)), fmt.Sprintf("number of used blobs should be %d, but is %d",
		uint(len(keep)), stats.Blobs.Used))
	rtest.Assert(t, uint64(sizeCompressed) == stats.Size.Used, "used size should be identical, but is %d vs %d", sizeCompressed, stats.Size.Used)
	// all of the small packfiles should have been repacked
	rtest.Assert(t, uint(numPackfiles) == stats.Packs.Repack,
		"the number repacked packfiles should be %d, but are %d", numPackfiles, stats.Packs.Repack)
}
