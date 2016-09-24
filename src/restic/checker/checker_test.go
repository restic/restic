package checker_test

import (
	"math/rand"
	"path/filepath"
	"sort"
	"testing"

	"restic"
	"restic/archiver"
	"restic/backend/mem"
	"restic/checker"
	"restic/repository"
	"restic/test"
)

var checkerTestData = filepath.Join("testdata", "checker-test-repo.tar.gz")

func collectErrors(f func(chan<- error, <-chan struct{})) (errs []error) {
	done := make(chan struct{})
	defer close(done)

	errChan := make(chan error)

	go f(errChan, done)

	for err := range errChan {
		errs = append(errs, err)
	}

	return errs
}

func checkPacks(chkr *checker.Checker) []error {
	return collectErrors(chkr.Packs)
}

func checkStruct(chkr *checker.Checker) []error {
	return collectErrors(chkr.Structure)
}

func checkData(chkr *checker.Checker) []error {
	return collectErrors(
		func(errCh chan<- error, done <-chan struct{}) {
			chkr.ReadData(nil, errCh, done)
		},
	)
}

func TestCheckRepo(t *testing.T) {
	repodir, cleanup := test.Env(t, checkerTestData)
	defer cleanup()

	repo := repository.TestOpenLocal(t, repodir)

	chkr := checker.New(repo)
	hints, errs := chkr.LoadIndex()
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %v: %v", len(errs), errs)
	}

	if len(hints) > 0 {
		t.Errorf("expected no hints, got %v: %v", len(hints), hints)
	}

	test.OKs(t, checkPacks(chkr))
	test.OKs(t, checkStruct(chkr))
}

func TestMissingPack(t *testing.T) {
	repodir, cleanup := test.Env(t, checkerTestData)
	defer cleanup()

	repo := repository.TestOpenLocal(t, repodir)

	packID := "657f7fb64f6a854fff6fe9279998ee09034901eded4e6db9bcee0e59745bbce6"
	test.OK(t, repo.Backend().Remove(restic.DataFile, packID))

	chkr := checker.New(repo)
	hints, errs := chkr.LoadIndex()
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %v: %v", len(errs), errs)
	}

	if len(hints) > 0 {
		t.Errorf("expected no hints, got %v: %v", len(hints), hints)
	}

	errs = checkPacks(chkr)

	test.Assert(t, len(errs) == 1,
		"expected exactly one error, got %v", len(errs))

	if err, ok := errs[0].(checker.PackError); ok {
		test.Equals(t, packID, err.ID.String())
	} else {
		t.Errorf("expected error returned by checker.Packs() to be PackError, got %v", err)
	}
}

func TestUnreferencedPack(t *testing.T) {
	repodir, cleanup := test.Env(t, checkerTestData)
	defer cleanup()

	repo := repository.TestOpenLocal(t, repodir)

	// index 3f1a only references pack 60e0
	indexID := "3f1abfcb79c6f7d0a3be517d2c83c8562fba64ef2c8e9a3544b4edaf8b5e3b44"
	packID := "60e0438dcb978ec6860cc1f8c43da648170ee9129af8f650f876bad19f8f788e"
	test.OK(t, repo.Backend().Remove(restic.IndexFile, indexID))

	chkr := checker.New(repo)
	hints, errs := chkr.LoadIndex()
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %v: %v", len(errs), errs)
	}

	if len(hints) > 0 {
		t.Errorf("expected no hints, got %v: %v", len(hints), hints)
	}

	errs = checkPacks(chkr)

	test.Assert(t, len(errs) == 1,
		"expected exactly one error, got %v", len(errs))

	if err, ok := errs[0].(checker.PackError); ok {
		test.Equals(t, packID, err.ID.String())
	} else {
		t.Errorf("expected error returned by checker.Packs() to be PackError, got %v", err)
	}
}

func TestUnreferencedBlobs(t *testing.T) {
	repodir, cleanup := test.Env(t, checkerTestData)
	defer cleanup()

	repo := repository.TestOpenLocal(t, repodir)

	snID := "51d249d28815200d59e4be7b3f21a157b864dc343353df9d8e498220c2499b02"
	test.OK(t, repo.Backend().Remove(restic.SnapshotFile, snID))

	unusedBlobsBySnapshot := restic.IDs{
		restic.TestParseID("58c748bbe2929fdf30c73262bd8313fe828f8925b05d1d4a87fe109082acb849"),
		restic.TestParseID("988a272ab9768182abfd1fe7d7a7b68967825f0b861d3b36156795832c772235"),
		restic.TestParseID("c01952de4d91da1b1b80bc6e06eaa4ec21523f4853b69dc8231708b9b7ec62d8"),
		restic.TestParseID("bec3a53d7dc737f9a9bee68b107ec9e8ad722019f649b34d474b9982c3a3fec7"),
		restic.TestParseID("2a6f01e5e92d8343c4c6b78b51c5a4dc9c39d42c04e26088c7614b13d8d0559d"),
		restic.TestParseID("18b51b327df9391732ba7aaf841a4885f350d8a557b2da8352c9acf8898e3f10"),
	}

	sort.Sort(unusedBlobsBySnapshot)

	chkr := checker.New(repo)
	hints, errs := chkr.LoadIndex()
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %v: %v", len(errs), errs)
	}

	if len(hints) > 0 {
		t.Errorf("expected no hints, got %v: %v", len(hints), hints)
	}

	test.OKs(t, checkPacks(chkr))
	test.OKs(t, checkStruct(chkr))

	blobs := chkr.UnusedBlobs()
	sort.Sort(blobs)

	test.Equals(t, unusedBlobsBySnapshot, blobs)
}

var checkerDuplicateIndexTestData = filepath.Join("testdata", "duplicate-packs-in-index-test-repo.tar.gz")

func TestDuplicatePacksInIndex(t *testing.T) {
	repodir, cleanup := test.Env(t, checkerDuplicateIndexTestData)
	defer cleanup()

	repo := repository.TestOpenLocal(t, repodir)

	chkr := checker.New(repo)
	hints, errs := chkr.LoadIndex()
	if len(hints) == 0 {
		t.Fatalf("did not get expected checker hints for duplicate packs in indexes")
	}

	found := false
	for _, hint := range hints {
		if _, ok := hint.(checker.ErrDuplicatePacks); ok {
			found = true
		} else {
			t.Errorf("got unexpected hint: %v", hint)
		}
	}

	if !found {
		t.Fatalf("did not find hint ErrDuplicatePacks")
	}

	if len(errs) > 0 {
		t.Errorf("expected no errors, got %v: %v", len(errs), errs)
	}
}

// errorBackend randomly modifies data after reading.
type errorBackend struct {
	restic.Backend
	ProduceErrors bool
}

func (b errorBackend) Load(h restic.Handle, p []byte, off int64) (int, error) {
	n, err := b.Backend.Load(h, p, off)

	if b.ProduceErrors {
		induceError(p)
	}
	return n, err
}

// induceError flips a bit in the slice.
func induceError(data []byte) {
	if rand.Float32() < 0.2 {
		return
	}

	pos := rand.Intn(len(data))
	data[pos] ^= 1
}

func TestCheckerModifiedData(t *testing.T) {
	be := mem.New()

	repository.TestUseLowSecurityKDFParameters(t)

	repo := repository.New(be)
	test.OK(t, repo.Init(test.TestPassword))

	arch := archiver.New(repo)
	_, id, err := arch.Snapshot(nil, []string{"."}, nil, nil)
	test.OK(t, err)
	t.Logf("archived as %v", id.Str())

	beError := &errorBackend{Backend: be}
	checkRepo := repository.New(beError)
	test.OK(t, checkRepo.SearchKey(test.TestPassword, 5))

	chkr := checker.New(checkRepo)

	hints, errs := chkr.LoadIndex()
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %v: %v", len(errs), errs)
	}

	if len(hints) > 0 {
		t.Errorf("expected no hints, got %v: %v", len(hints), hints)
	}

	beError.ProduceErrors = true
	errFound := false
	for _, err := range checkPacks(chkr) {
		t.Logf("pack error: %v", err)
	}

	for _, err := range checkStruct(chkr) {
		t.Logf("struct error: %v", err)
	}

	for _, err := range checkData(chkr) {
		t.Logf("struct error: %v", err)
		errFound = true
	}

	if !errFound {
		t.Fatal("no error found, checker is broken")
	}
}
