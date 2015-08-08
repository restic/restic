package checker_test

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/checker"
	"github.com/restic/restic/repository"
	. "github.com/restic/restic/test"
)

var checkerTestData = filepath.Join("testdata", "checker-test-repo.tar.gz")

func list(repo *repository.Repository, t backend.Type) (IDs []string) {
	done := make(chan struct{})
	defer close(done)

	for id := range repo.List(t, done) {
		IDs = append(IDs, id.String())
	}

	return IDs
}

func checkPacks(chkr *checker.Checker) (errs []error) {
	done := make(chan struct{})
	defer close(done)

	errChan := make(chan error)

	go chkr.Packs(errChan, done)

	for err := range errChan {
		errs = append(errs, err)
	}

	return errs
}

func checkStruct(chkr *checker.Checker) (errs []error) {
	done := make(chan struct{})
	defer close(done)

	errChan := make(chan error)

	go chkr.Structure(errChan, done)

	for err := range errChan {
		errs = append(errs, err)
	}

	return errs
}

func TestCheckRepo(t *testing.T) {
	WithTestEnvironment(t, checkerTestData, func(repodir string) {
		repo := OpenLocalRepo(t, repodir)

		chkr := checker.New(repo)
		OK(t, chkr.LoadIndex())
		OKs(t, checkPacks(chkr))
		OKs(t, checkStruct(chkr))
	})
}

func TestMissingPack(t *testing.T) {
	WithTestEnvironment(t, checkerTestData, func(repodir string) {
		repo := OpenLocalRepo(t, repodir)

		packID := "657f7fb64f6a854fff6fe9279998ee09034901eded4e6db9bcee0e59745bbce6"
		OK(t, repo.Backend().Remove(backend.Data, packID))

		chkr := checker.New(repo)
		OK(t, chkr.LoadIndex())
		errs := checkPacks(chkr)

		Assert(t, len(errs) == 1,
			"expected exactly one error, got %v", len(errs))

		if err, ok := errs[0].(checker.PackError); ok {
			Equals(t, packID, err.ID.String())
		} else {
			t.Errorf("expected error returned by checker.Packs() to be PackError, got %v", err)
		}
	})
}

func TestUnreferencedPack(t *testing.T) {
	WithTestEnvironment(t, checkerTestData, func(repodir string) {
		repo := OpenLocalRepo(t, repodir)

		// index 3f1a only references pack 60e0
		indexID := "3f1abfcb79c6f7d0a3be517d2c83c8562fba64ef2c8e9a3544b4edaf8b5e3b44"
		packID := "60e0438dcb978ec6860cc1f8c43da648170ee9129af8f650f876bad19f8f788e"
		OK(t, repo.Backend().Remove(backend.Index, indexID))

		chkr := checker.New(repo)
		OK(t, chkr.LoadIndex())
		errs := checkPacks(chkr)

		Assert(t, len(errs) == 1,
			"expected exactly one error, got %v", len(errs))

		if err, ok := errs[0].(checker.PackError); ok {
			Equals(t, packID, err.ID.String())
		} else {
			t.Errorf("expected error returned by checker.Packs() to be PackError, got %v", err)
		}
	})
}

func TestUnreferencedBlobs(t *testing.T) {
	WithTestEnvironment(t, checkerTestData, func(repodir string) {
		repo := OpenLocalRepo(t, repodir)

		snID := "51d249d28815200d59e4be7b3f21a157b864dc343353df9d8e498220c2499b02"
		OK(t, repo.Backend().Remove(backend.Snapshot, snID))

		unusedBlobsBySnapshot := backend.IDs{
			ParseID("58c748bbe2929fdf30c73262bd8313fe828f8925b05d1d4a87fe109082acb849"),
			ParseID("988a272ab9768182abfd1fe7d7a7b68967825f0b861d3b36156795832c772235"),
			ParseID("c01952de4d91da1b1b80bc6e06eaa4ec21523f4853b69dc8231708b9b7ec62d8"),
			ParseID("bec3a53d7dc737f9a9bee68b107ec9e8ad722019f649b34d474b9982c3a3fec7"),
			ParseID("2a6f01e5e92d8343c4c6b78b51c5a4dc9c39d42c04e26088c7614b13d8d0559d"),
			ParseID("18b51b327df9391732ba7aaf841a4885f350d8a557b2da8352c9acf8898e3f10"),
		}

		sort.Sort(unusedBlobsBySnapshot)

		chkr := checker.New(repo)
		OK(t, chkr.LoadIndex())
		OKs(t, checkPacks(chkr))
		OKs(t, checkStruct(chkr))

		blobs := chkr.UnusedBlobs()
		sort.Sort(blobs)

		Equals(t, unusedBlobsBySnapshot, blobs)
	})
}
