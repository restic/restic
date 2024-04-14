package repository_test

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/checker"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"github.com/restic/restic/internal/ui/progress"
)

func listIndex(t *testing.T, repo restic.Lister) restic.IDSet {
	return listFiles(t, repo, restic.IndexFile)
}

func testRebuildIndex(t *testing.T, readAllPacks bool, damage func(t *testing.T, repo *repository.Repository)) {
	repo := repository.TestRepository(t).(*repository.Repository)
	createRandomBlobs(t, repo, 4, 0.5, true)
	createRandomBlobs(t, repo, 5, 0.5, true)
	indexes := listIndex(t, repo)
	t.Logf("old indexes %v", indexes)

	damage(t, repo)

	repo = repository.TestOpenBackend(t, repo.Backend()).(*repository.Repository)
	rtest.OK(t, repository.RepairIndex(context.TODO(), repo, repository.RepairIndexOptions{
		ReadAllPacks: readAllPacks,
	}, &progress.NoopPrinter{}))

	newIndexes := listIndex(t, repo)
	old := indexes.Intersect(newIndexes)
	rtest.Assert(t, len(old) == 0, "expected old indexes to be removed, found %v", old)

	checker.TestCheckRepo(t, repo, true)
}

func TestRebuildIndex(t *testing.T) {
	for _, test := range []struct {
		name   string
		damage func(t *testing.T, repo *repository.Repository)
	}{
		{
			"valid index",
			func(t *testing.T, repo *repository.Repository) {},
		},
		{
			"damaged index",
			func(t *testing.T, repo *repository.Repository) {
				index := listIndex(t, repo).List()[0]
				replaceFile(t, repo, backend.Handle{Type: restic.IndexFile, Name: index.String()}, func(b []byte) []byte {
					b[0] ^= 0xff
					return b
				})
			},
		},
		{
			"missing index",
			func(t *testing.T, repo *repository.Repository) {
				index := listIndex(t, repo).List()[0]
				rtest.OK(t, repo.Backend().Remove(context.TODO(), backend.Handle{Type: restic.IndexFile, Name: index.String()}))
			},
		},
		{
			"missing pack",
			func(t *testing.T, repo *repository.Repository) {
				pack := listPacks(t, repo).List()[0]
				rtest.OK(t, repo.Backend().Remove(context.TODO(), backend.Handle{Type: restic.PackFile, Name: pack.String()}))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			testRebuildIndex(t, false, test.damage)
			testRebuildIndex(t, true, test.damage)
		})
	}
}
