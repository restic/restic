package checker_test

import (
	"path/filepath"
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

func TestCheckRepo(t *testing.T) {
	WithTestEnvironment(t, checkerTestData, func(repodir string) {
		repo := OpenLocalRepo(t, repodir)

		checker := checker.New(repo)
		OK(t, checker.LoadIndex())
	})
}
