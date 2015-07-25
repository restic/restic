package checker_test

import (
	"testing"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/checker"

	. "github.com/restic/restic/test"
)

var findPackTests = []struct {
	blobIDs backend.IDs
	packIDs backend.IDSet
}{
	{
		backend.IDs{
			ParseID("534f211b4fc2cf5b362a24e8eba22db5372a75b7e974603ff9263f5a471760f4"),
			ParseID("51aa04744b518c6a85b4e7643cfa99d58789c2a6ca2a3fda831fa3032f28535c"),
			ParseID("454515bca5f4f60349a527bd814cc2681bc3625716460cc6310771c966d8a3bf"),
			ParseID("c01952de4d91da1b1b80bc6e06eaa4ec21523f4853b69dc8231708b9b7ec62d8"),
		},
		backend.IDSet{
			ParseID("19a731a515618ec8b75fc0ff3b887d8feb83aef1001c9899f6702761142ed068"): struct{}{},
			ParseID("657f7fb64f6a854fff6fe9279998ee09034901eded4e6db9bcee0e59745bbce6"): struct{}{},
		},
	},
}

func TestRepackerFindPacks(t *testing.T) {
	WithTestEnvironment(t, checkerTestData, func(repodir string) {
		repo := OpenLocalRepo(t, repodir)

		OK(t, repo.LoadIndex())

		for _, test := range findPackTests {
			packIDs, err := checker.FindPacksforBlobs(repo, test.blobIDs)
			OK(t, err)
			Equals(t, test.packIDs, packIDs)
		}
	})
}

func TestRepackBlobs(t *testing.T) {
	WithTestEnvironment(t, checkerTestData, func(repodir string) {
		repo := OpenLocalRepo(t, repodir)
		OK(t, repo.LoadIndex())

		repo.Backend().Remove(backend.Snapshot, "c2b53c5e6a16db92fbb9aa08bd2794c58b379d8724d661ee30d20898bdfdff22")

		unusedBlobs := backend.IDSet{
			ParseID("5714f7274a8aa69b1692916739dc3835d09aac5395946b8ec4f58e563947199a"): struct{}{},
			ParseID("08d0444e9987fa6e35ce4232b2b71473e1a8f66b2f9664cc44dc57aad3c5a63a"): struct{}{},
			ParseID("356493f0b00a614d36c698591bbb2b1d801932d85328c1f508019550034549fc"): struct{}{},
			ParseID("b8a6bcdddef5c0f542b4648b2ef79bc0ed4377d4109755d2fb78aff11e042663"): struct{}{},
		}

		chkr := checker.New(repo)
		_, errs := chkr.LoadIndex()
		OKs(t, errs)

		errs = checkStruct(chkr)
		OKs(t, errs)

		list := backend.NewIDSet(chkr.UnusedBlobs()...)
		if !unusedBlobs.Equals(list) {
			t.Fatalf("expected unused blobs:\n  %v\ngot:\n  %v", unusedBlobs, list)
		}

		// repacker := checker.NewRepacker(repo, repo, repackBlobIDs)
		// OK(t, repacker.Repack())

		// err := checker.RepackBlobs(repo, repo, repackBlobIDs)
		// OK(t, err)

		// newPackIDs, err := checker.FindPacksforBlobs(repo, repackBlobIDs)
		// OK(t, err)
		// fmt.Printf("new pack IDs: %v\n", newPackIDs)

		chkr = checker.New(repo)
		_, errs = chkr.LoadIndex()
		OKs(t, errs)
		OKs(t, checkPacks(chkr))
		OKs(t, checkStruct(chkr))

		blobs := chkr.UnusedBlobs()
		Assert(t, len(blobs) == 0,
			"expected zero unused blobs, got %v", blobs)
	})
}
