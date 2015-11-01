package checker_test

import (
	"testing"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/checker"

	. "github.com/restic/restic/test"
)

var findPackTests = []struct {
	blobIDs backend.IDSet
	packIDs backend.IDSet
}{
	{
		backend.IDSet{
			ParseID("534f211b4fc2cf5b362a24e8eba22db5372a75b7e974603ff9263f5a471760f4"): struct{}{},
			ParseID("51aa04744b518c6a85b4e7643cfa99d58789c2a6ca2a3fda831fa3032f28535c"): struct{}{},
			ParseID("454515bca5f4f60349a527bd814cc2681bc3625716460cc6310771c966d8a3bf"): struct{}{},
			ParseID("c01952de4d91da1b1b80bc6e06eaa4ec21523f4853b69dc8231708b9b7ec62d8"): struct{}{},
		},
		backend.IDSet{
			ParseID("19a731a515618ec8b75fc0ff3b887d8feb83aef1001c9899f6702761142ed068"): struct{}{},
			ParseID("657f7fb64f6a854fff6fe9279998ee09034901eded4e6db9bcee0e59745bbce6"): struct{}{},
		},
	},
}

var findBlobTests = []struct {
	packIDs backend.IDSet
	blobIDs backend.IDSet
}{
	{
		backend.IDSet{
			ParseID("60e0438dcb978ec6860cc1f8c43da648170ee9129af8f650f876bad19f8f788e"): struct{}{},
		},
		backend.IDSet{
			ParseID("356493f0b00a614d36c698591bbb2b1d801932d85328c1f508019550034549fc"): struct{}{},
			ParseID("b8a6bcdddef5c0f542b4648b2ef79bc0ed4377d4109755d2fb78aff11e042663"): struct{}{},
			ParseID("5714f7274a8aa69b1692916739dc3835d09aac5395946b8ec4f58e563947199a"): struct{}{},
			ParseID("b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c"): struct{}{},
			ParseID("08d0444e9987fa6e35ce4232b2b71473e1a8f66b2f9664cc44dc57aad3c5a63a"): struct{}{},
		},
	},
	{
		backend.IDSet{
			ParseID("60e0438dcb978ec6860cc1f8c43da648170ee9129af8f650f876bad19f8f788e"): struct{}{},
			ParseID("ff7e12cd66d896b08490e787d1915c641e678d7e6b4a00e60db5d13054f4def4"): struct{}{},
		},
		backend.IDSet{
			ParseID("356493f0b00a614d36c698591bbb2b1d801932d85328c1f508019550034549fc"): struct{}{},
			ParseID("b8a6bcdddef5c0f542b4648b2ef79bc0ed4377d4109755d2fb78aff11e042663"): struct{}{},
			ParseID("5714f7274a8aa69b1692916739dc3835d09aac5395946b8ec4f58e563947199a"): struct{}{},
			ParseID("b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c"): struct{}{},
			ParseID("08d0444e9987fa6e35ce4232b2b71473e1a8f66b2f9664cc44dc57aad3c5a63a"): struct{}{},
			ParseID("aa79d596dbd4c863e5400deaca869830888fe1ce9f51b4a983f532c77f16a596"): struct{}{},
			ParseID("b2396c92781307111accf2ebb1cd62b58134b744d90cb6f153ca456a98dc3e76"): struct{}{},
			ParseID("5249af22d3b2acd6da8048ac37b2a87fa346fabde55ed23bb866f7618843c9fe"): struct{}{},
			ParseID("f41c2089a9d58a4b0bf39369fa37588e6578c928aea8e90a4490a6315b9905c1"): struct{}{},
		},
	},
}

func TestRepackerFindPacks(t *testing.T) {
	WithTestEnvironment(t, checkerTestData, func(repodir string) {
		repo := OpenLocalRepo(t, repodir)

		OK(t, repo.LoadIndex())

		for _, test := range findPackTests {
			packIDs, err := checker.FindPacksForBlobs(repo, test.blobIDs)
			OK(t, err)
			Equals(t, test.packIDs, packIDs)
		}

		for _, test := range findBlobTests {
			blobs, err := checker.FindBlobsForPacks(repo, test.packIDs)
			OK(t, err)

			Assert(t, test.blobIDs.Equals(blobs),
				"list of blobs for packs %v does not match, expected:\n  %v\ngot:\n  %v",
				test.packIDs, test.blobIDs, blobs)
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

		repacker := checker.NewRepacker(repo, repo, unusedBlobs)
		OK(t, repacker.Repack())

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
