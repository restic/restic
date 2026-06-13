package repository_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	backendtest "github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
)

func listBlobs(repo restic.Repository) restic.BlobSet {
	blobs := restic.NewBlobSet()
	_ = repo.ListBlobs(context.TODO(), func(pb restic.PackBlob) {
		blobs.Insert(pb.Handle())
	})
	return blobs
}

func replaceFile(t *testing.T, be backend.Backend, h backend.Handle, damage func([]byte) []byte) {
	buf, err := backendtest.LoadAll(context.TODO(), be, h)
	rtest.OK(t, err)
	buf = damage(buf)
	rtest.OK(t, be.Remove(context.TODO(), h))
	rtest.OK(t, be.Save(context.TODO(), h, backend.NewByteReader(buf, be.Hasher())))
}

func TestRepairBrokenPack(t *testing.T) {
	repository.TestAllVersions(t, testRepairBrokenPack)
}

func testRepairBrokenPack(t *testing.T, version uint) {
	tests := []struct {
		name   string
		damage func(t *testing.T, random *rand.Rand, repo *repository.Repository, be backend.Backend, packsBefore restic.IDSet) (restic.IDSet, restic.BlobSet)
	}{
		{
			"valid pack",
			func(t *testing.T, random *rand.Rand, repo *repository.Repository, be backend.Backend, packsBefore restic.IDSet) (restic.IDSet, restic.BlobSet) {
				return packsBefore, restic.NewBlobSet()
			},
		},
		{
			"broken pack",
			func(t *testing.T, random *rand.Rand, repo *repository.Repository, be backend.Backend, packsBefore restic.IDSet) (restic.IDSet, restic.BlobSet) {
				wrongBlob := createRandomWrongBlob(t, random, repo)
				damagedPacks := findPacksForBlobs(t, repo, restic.NewBlobSet(wrongBlob))
				return damagedPacks, restic.NewBlobSet(wrongBlob)
			},
		},
		{
			"partially broken pack",
			func(t *testing.T, random *rand.Rand, repo *repository.Repository, be backend.Backend, packsBefore restic.IDSet) (restic.IDSet, restic.BlobSet) {
				// damage one of the pack files
				damagedID := packsBefore.List()[0]
				replaceFile(t, be, backend.Handle{Type: backend.PackFile, Name: damagedID.String()},
					func(buf []byte) []byte {
						buf[0] ^= 0xff
						return buf
					})

				// find blob that starts at offset 0
				var damagedBlob restic.BlobHandle
				for _, blob := range repository.BlobsInPack(repo, damagedID) {
					if blob.Offset == 0 {
						damagedBlob = blob.BlobHandle
						break
					}
				}

				return restic.NewIDSet(damagedID), restic.NewBlobSet(damagedBlob)
			},
		}, {
			"truncated pack",
			func(t *testing.T, random *rand.Rand, repo *repository.Repository, be backend.Backend, packsBefore restic.IDSet) (restic.IDSet, restic.BlobSet) {
				// damage one of the pack files
				damagedID := packsBefore.List()[0]
				replaceFile(t, be, backend.Handle{Type: backend.PackFile, Name: damagedID.String()},
					func(buf []byte) []byte {
						buf = buf[0:10]
						return buf
					})

				// all blobs in the file are broken
				damagedBlobs := restic.NewBlobSet()
				rtest.OK(t, repo.ListBlobs(context.TODO(), func(pb restic.PackBlob) {
					if pb.PackID().Equal(damagedID) {
						damagedBlobs.Insert(pb.Handle())
					}
				}))
				return restic.NewIDSet(damagedID), damagedBlobs
			},
		}, {
			"unindexed pack",
			func(t *testing.T, random *rand.Rand, repo *repository.Repository, be backend.Backend, packsBefore restic.IDSet) (restic.IDSet, restic.BlobSet) {
				// remove one pack file from the index
				unindexID := packsBefore.List()[0]
				h := backend.Handle{Type: backend.PackFile, Name: unindexID.String()}

				buf, err := backendtest.LoadAll(context.TODO(), be, h)
				rtest.OK(t, err)
				rtest.OK(t, be.Remove(context.TODO(), h))
				rtest.OK(t, repository.RepairIndex(context.TODO(), repo, repository.RepairIndexOptions{}, progress.NewNoopPrinter()))

				rtest.OK(t, be.Save(context.TODO(), h, backend.NewByteReader(buf, be.Hasher())))

				return restic.NewIDSet(unindexID), restic.NewBlobSet()
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// disable verification to allow adding corrupted blobs to the repository
			repo, be := repository.TestRepositoryWithBackend(t, nil, version, repository.Options{NoExtraVerify: true})

			seed := time.Now().UnixNano()
			random := rand.New(rand.NewSource(seed))
			t.Logf("rand seed is %v", seed)

			createRandomBlobs(t, random, repo, 5, 0.7, true)
			packsBefore := listPacks(t, repo)
			blobsBefore := listBlobs(repo)

			toRepair, damagedBlobs := test.damage(t, random, repo, be, packsBefore)

			rtest.OK(t, repository.RepairPacks(context.TODO(), repo, toRepair, progress.NewNoopPrinter()))
			// reload index
			rtest.OK(t, repo.LoadIndex(context.TODO(), nil))

			packsAfter := listPacks(t, repo)
			blobsAfter := listBlobs(repo)

			rtest.Assert(t, len(packsAfter.Intersect(toRepair)) == 0, "some damaged packs were not removed")
			rtest.Assert(t, len(packsBefore.Sub(toRepair).Sub(packsAfter)) == 0, "not-damaged packs were removed")
			rtest.Assert(t, blobsBefore.Sub(damagedBlobs).Equals(blobsAfter), "diverging blob lists")
		})
	}
}
