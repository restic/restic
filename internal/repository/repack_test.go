package repository_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
)

func randomSize(random *rand.Rand, min, max int) int {
	return random.Intn(max-min) + min
}

func createRandomBlobs(t testing.TB, random *rand.Rand, repo restic.Repository, blobs int, pData float32, smallBlobs bool) {
	// two loops to allow creating multiple pack files
	for blobs > 0 {
		rtest.OK(t, repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaver) error {
			for blobs > 0 {
				blobs--
				var (
					tpe    restic.BlobType
					length int
				)

				if random.Float32() < pData {
					tpe = restic.DataBlob
					if smallBlobs {
						length = randomSize(random, 1*1024, 20*1024) // 1KiB to 20KiB of data
					} else {
						length = randomSize(random, 10*1024, 1024*1024) // 10KiB to 1MiB of data
					}
				} else {
					tpe = restic.TreeBlob
					length = randomSize(random, 1*1024, 20*1024) // 1KiB to 20KiB
				}

				buf := make([]byte, length)
				random.Read(buf)

				id, exists, _, err := uploader.SaveBlob(ctx, tpe, buf, restic.ID{}, false)
				if err != nil {
					t.Fatalf("SaveFrom() error %v", err)
				}

				if exists {
					t.Errorf("duplicate blob %v/%v ignored", id, restic.DataBlob)
					continue
				}

				if rand.Float32() < 0.2 {
					break
				}
			}
			return nil
		}))
	}
}

func createRandomWrongBlob(t testing.TB, random *rand.Rand, repo restic.Repository) restic.BlobHandle {
	length := randomSize(random, 10*1024, 1024*1024) // 10KiB to 1MiB of data
	buf := make([]byte, length)
	random.Read(buf)
	id := restic.Hash(buf)
	// invert first data byte
	buf[0] ^= 0xff

	rtest.OK(t, repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaver) error {
		_, _, _, err := uploader.SaveBlob(ctx, restic.DataBlob, buf, id, false)
		return err
	}))
	return restic.BlobHandle{ID: id, Type: restic.DataBlob}
}

// selectBlobs splits the list of all blobs randomly into two lists. A blob
// will be contained in the firstone with probability p.
func selectBlobs(t *testing.T, random *rand.Rand, repo restic.Repository, p float32) (list1, list2 restic.BlobSet) {
	list1 = restic.NewBlobSet()
	list2 = restic.NewBlobSet()

	blobs := restic.NewBlobSet()

	err := repo.List(context.TODO(), restic.PackFile, func(id restic.ID, size int64) error {
		entries, _, err := repo.ListPack(context.TODO(), id, size)
		if err != nil {
			t.Fatalf("error listing pack %v: %v", id, err)
		}

		for _, entry := range entries {
			h := restic.BlobHandle{ID: entry.ID, Type: entry.Type}
			if blobs.Has(h) {
				t.Errorf("ignoring duplicate blob %v", h)
				return nil
			}
			blobs.Insert(h)

			if random.Float32() <= p {
				list1.Insert(restic.BlobHandle{ID: entry.ID, Type: entry.Type})
			} else {
				list2.Insert(restic.BlobHandle{ID: entry.ID, Type: entry.Type})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return list1, list2
}

func listPacks(t *testing.T, repo restic.Lister) restic.IDSet {
	return listFiles(t, repo, restic.PackFile)
}

func listFiles(t *testing.T, repo restic.Lister, tpe backend.FileType) restic.IDSet {
	list := restic.NewIDSet()
	err := repo.List(context.TODO(), tpe, func(id restic.ID, size int64) error {
		list.Insert(id)
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}

	return list
}

func findPacksForBlobs(t *testing.T, repo restic.Repository, blobs restic.BlobSet) restic.IDSet {
	packs := restic.NewIDSet()

	for h := range blobs {
		list := repo.LookupBlob(h.Type, h.ID)
		if len(list) == 0 {
			t.Fatal("Failed to find blob", h.ID.Str(), "with type", h.Type)
		}

		for _, pb := range list {
			packs.Insert(pb.PackID)
		}
	}

	return packs
}

func repack(t *testing.T, repo restic.Repository, be backend.Backend, packs restic.IDSet, blobs restic.BlobSet) {
	repackedBlobs, err := repository.Repack(context.TODO(), repo, repo, packs, blobs, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	for id := range repackedBlobs {
		err = be.Remove(context.TODO(), backend.Handle{Type: restic.PackFile, Name: id.String()})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func rebuildAndReloadIndex(t *testing.T, repo *repository.Repository) {
	rtest.OK(t, repository.RepairIndex(context.TODO(), repo, repository.RepairIndexOptions{
		ReadAllPacks: true,
	}, &progress.NoopPrinter{}))

	rtest.OK(t, repo.LoadIndex(context.TODO(), nil))
}

func TestRepack(t *testing.T) {
	repository.TestAllVersions(t, testRepack)
}

func testRepack(t *testing.T, version uint) {
	repo, _, be := repository.TestRepositoryWithVersion(t, version)

	seed := time.Now().UnixNano()
	random := rand.New(rand.NewSource(seed))
	t.Logf("rand seed is %v", seed)

	// add a small amount of blobs twice to create multiple pack files
	createRandomBlobs(t, random, repo, 10, 0.7, false)
	createRandomBlobs(t, random, repo, 10, 0.7, false)

	packsBefore := listPacks(t, repo)

	// Running repack on empty ID sets should not do anything at all.
	repack(t, repo, be, nil, nil)

	packsAfter := listPacks(t, repo)

	if !packsAfter.Equals(packsBefore) {
		t.Fatalf("packs are not equal, Repack modified something. Before:\n  %v\nAfter:\n  %v",
			packsBefore, packsAfter)
	}

	removeBlobs, keepBlobs := selectBlobs(t, random, repo, 0.2)

	removePacks := findPacksForBlobs(t, repo, removeBlobs)

	repack(t, repo, be, removePacks, keepBlobs)
	rebuildAndReloadIndex(t, repo)

	packsAfter = listPacks(t, repo)
	for id := range removePacks {
		if packsAfter.Has(id) {
			t.Errorf("pack %v still present although it should have been repacked and removed", id.Str())
		}
	}

	for h := range keepBlobs {
		list := repo.LookupBlob(h.Type, h.ID)
		if len(list) == 0 {
			t.Errorf("unable to find blob %v in repo", h.ID.Str())
			continue
		}

		if len(list) != 1 {
			t.Errorf("expected one pack in the list, got: %v", list)
			continue
		}

		pb := list[0]

		if removePacks.Has(pb.PackID) {
			t.Errorf("lookup returned pack ID %v that should've been removed", pb.PackID)
		}
	}

	for h := range removeBlobs {
		if _, found := repo.LookupBlobSize(h.Type, h.ID); found {
			t.Errorf("blob %v still contained in the repo", h)
		}
	}
}

func TestRepackCopy(t *testing.T) {
	repository.TestAllVersions(t, testRepackCopy)
}

type oneConnectionRepo struct {
	restic.Repository
}

func (r oneConnectionRepo) Connections() uint {
	return 1
}

func testRepackCopy(t *testing.T, version uint) {
	repo, _, _ := repository.TestRepositoryWithVersion(t, version)
	dstRepo, _, _ := repository.TestRepositoryWithVersion(t, version)

	// test with minimal possible connection count
	repoWrapped := &oneConnectionRepo{repo}
	dstRepoWrapped := &oneConnectionRepo{dstRepo}

	seed := time.Now().UnixNano()
	random := rand.New(rand.NewSource(seed))
	t.Logf("rand seed is %v", seed)

	// add a small amount of blobs twice to create multiple pack files
	createRandomBlobs(t, random, repo, 10, 0.7, false)
	createRandomBlobs(t, random, repo, 10, 0.7, false)

	_, keepBlobs := selectBlobs(t, random, repo, 0.2)
	copyPacks := findPacksForBlobs(t, repo, keepBlobs)

	_, err := repository.Repack(context.TODO(), repoWrapped, dstRepoWrapped, copyPacks, keepBlobs, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rebuildAndReloadIndex(t, dstRepo)

	for h := range keepBlobs {
		list := dstRepo.LookupBlob(h.Type, h.ID)
		if len(list) == 0 {
			t.Errorf("unable to find blob %v in repo", h.ID.Str())
			continue
		}

		if len(list) != 1 {
			t.Errorf("expected one pack in the list, got: %v", list)
			continue
		}
	}
}

func TestRepackWrongBlob(t *testing.T) {
	repository.TestAllVersions(t, testRepackWrongBlob)
}

func testRepackWrongBlob(t *testing.T, version uint) {
	// disable verification to allow adding corrupted blobs to the repository
	repo, _ := repository.TestRepositoryWithBackend(t, nil, version, repository.Options{NoExtraVerify: true})

	seed := time.Now().UnixNano()
	random := rand.New(rand.NewSource(seed))
	t.Logf("rand seed is %v", seed)

	createRandomBlobs(t, random, repo, 5, 0.7, false)
	createRandomWrongBlob(t, random, repo)

	// just keep all blobs, but also rewrite every pack
	_, keepBlobs := selectBlobs(t, random, repo, 0)
	rewritePacks := findPacksForBlobs(t, repo, keepBlobs)

	_, err := repository.Repack(context.TODO(), repo, repo, rewritePacks, keepBlobs, nil, nil)
	if err == nil {
		t.Fatal("expected repack to fail but got no error")
	}
	t.Logf("found expected error: %v", err)
}

func TestRepackBlobFallback(t *testing.T) {
	repository.TestAllVersions(t, testRepackBlobFallback)
}

func testRepackBlobFallback(t *testing.T, version uint) {
	// disable verification to allow adding corrupted blobs to the repository
	repo, _ := repository.TestRepositoryWithBackend(t, nil, version, repository.Options{NoExtraVerify: true})

	seed := time.Now().UnixNano()
	random := rand.New(rand.NewSource(seed))
	t.Logf("rand seed is %v", seed)

	length := randomSize(random, 10*1024, 1024*1024) // 10KiB to 1MiB of data
	buf := make([]byte, length)
	random.Read(buf)
	id := restic.Hash(buf)

	// corrupted copy
	modbuf := make([]byte, len(buf))
	copy(modbuf, buf)
	// invert first data byte
	modbuf[0] ^= 0xff

	// create pack with broken copy
	rtest.OK(t, repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaver) error {
		_, _, _, err := uploader.SaveBlob(ctx, restic.DataBlob, modbuf, id, false)
		return err
	}))

	// find pack with damaged blob
	keepBlobs := restic.NewBlobSet(restic.BlobHandle{Type: restic.DataBlob, ID: id})
	rewritePacks := findPacksForBlobs(t, repo, keepBlobs)

	// create pack with valid copy
	rtest.OK(t, repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaver) error {
		_, _, _, err := uploader.SaveBlob(ctx, restic.DataBlob, buf, id, true)
		return err
	}))

	// repack must fallback to valid copy
	_, err := repository.Repack(context.TODO(), repo, repo, rewritePacks, keepBlobs, nil, nil)
	rtest.OK(t, err)

	keepBlobs = restic.NewBlobSet(restic.BlobHandle{Type: restic.DataBlob, ID: id})
	packs := findPacksForBlobs(t, repo, keepBlobs)
	rtest.Assert(t, len(packs) == 3, "unexpected number of copies: %v", len(packs))
}
