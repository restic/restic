package index_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/index"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

var repoFixture = filepath.Join("..", "repository", "testdata", "test-repo.tar.gz")

func TestRepositoryForAllIndexes(t *testing.T) {
	repodir, cleanup := rtest.Env(t, repoFixture)
	defer cleanup()

	repo := repository.TestOpenLocal(t, repodir)

	expectedIndexIDs := restic.NewIDSet()
	rtest.OK(t, repo.List(context.TODO(), restic.IndexFile, func(id restic.ID, size int64) error {
		expectedIndexIDs.Insert(id)
		return nil
	}))

	// check that all expected indexes are loaded without errors
	indexIDs := restic.NewIDSet()
	var indexErr error
	rtest.OK(t, index.ForAllIndexes(context.TODO(), repo, func(id restic.ID, index *index.Index, oldFormat bool, err error) error {
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

	err := index.ForAllIndexes(context.TODO(), repo, func(id restic.ID, index *index.Index, oldFormat bool, err error) error {
		return iterErr
	})

	rtest.Equals(t, iterErr, err)
}
