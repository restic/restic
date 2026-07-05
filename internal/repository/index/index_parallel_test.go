package index_test

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/repository/crypto"
	"github.com/restic/restic/internal/repository/index"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestRepositoryForAllIndexes(t *testing.T) {
	originalFull := index.Full
	defer func() {
		index.Full = originalFull
	}()
	index.Full = func(*index.Index) bool { return true }

	repo, unpacked, _ := repository.TestRepositoryWithVersion(t, restic.StableRepoVersion)

	mi := index.NewMasterIndex()
	for range 3 {
		packID := restic.NewRandomID()
		blob := pack.Blob{
			BlobHandle: restic.NewRandomBlobHandle(),
			Length:     uint(crypto.CiphertextLength(10)),
			Offset:     0,
		}
		rtest.OK(t, mi.StorePack(context.TODO(), packID, pack.Blobs{blob}, unpacked))
		rtest.OK(t, mi.Flush(context.TODO(), unpacked))
	}

	expectedIndexIDs := restic.NewIDSet()
	rtest.OK(t, repo.List(context.TODO(), restic.IndexFile, func(id restic.ID, size int64) error {
		expectedIndexIDs.Insert(id)
		return nil
	}))
	rtest.Assert(t, len(expectedIndexIDs) > 1, "test repo should have multiple indexes")

	// check that all expected indexes are loaded without errors
	indexIDs := restic.NewIDSet()
	var indexErr error
	rtest.OK(t, index.ForAllIndexes(context.TODO(), repo, repo, func(id restic.ID, index *index.Index, err error) error {
		if err != nil {
			indexErr = err
		}
		indexIDs.Insert(id)
		return nil
	}))
	rtest.OK(t, indexErr)
	rtest.Equals(t, expectedIndexIDs, indexIDs)

	// must failed with the returned error
	iterErr := errors.New("error to pass upwards")

	err := index.ForAllIndexes(context.TODO(), repo, repo, func(id restic.ID, index *index.Index, err error) error {
		return iterErr
	})

	rtest.Equals(t, iterErr, err)
}
