package repository_test

import (
	"context"
	"testing"

	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestAllIndexBlobs(t *testing.T) {
	repo, _, _ := repository.TestRepositoryWithVersion(t, 0)

	want := restic.NewBlobSet()
	rtest.OK(t, repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		for i := range 5 {
			data := []byte{byte('a' + i)}
			id, _, _, err := uploader.SaveBlob(ctx, restic.DataBlob, data, restic.ID{}, false)
			rtest.OK(t, err)
			want.Insert(restic.BlobHandle{Type: restic.DataBlob, ID: id})
		}
		return nil
	}))

	rtest.OK(t, repo.LoadIndex(context.TODO(), nil))

	fromMaster := restic.NewBlobSet()
	rtest.OK(t, repo.ListBlobs(context.TODO(), func(pb restic.PackedBlob) {
		fromMaster.Insert(pb.BlobHandle)
	}))
	rtest.Equals(t, want, fromMaster)

	fromStream := restic.NewBlobSet()
	for entry := range repository.AllIndexBlobs(context.TODO(), repo, repo) {
		if entry.Error != nil {
			t.Fatalf("unexpected error: %v", entry.Error)
		}
		fromStream.Insert(entry.Handle)
	}
	rtest.Equals(t, want, fromStream)
}

func TestAllIndexBlobsEarlyStop(t *testing.T) {
	repo, _, _ := repository.TestRepositoryWithVersion(t, 0)

	rtest.OK(t, repo.WithBlobUploader(context.TODO(), func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		for range 5 {
			_, _, _, err := uploader.SaveBlob(ctx, restic.DataBlob, []byte("test"), restic.ID{}, false)
			rtest.OK(t, err)
		}
		return nil
	}))

	var count int
	for entry := range repository.AllIndexBlobs(context.TODO(), repo, repo) {
		rtest.Assert(t, entry.Error == nil, "unexpected error after early stop: %v", entry.Error)
		count++
		break
	}
	rtest.Equals(t, 1, count)
}
