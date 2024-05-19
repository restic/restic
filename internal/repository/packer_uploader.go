package repository

import (
	"context"

	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

// savePacker implements saving a pack in the repository.
type savePacker interface {
	savePacker(ctx context.Context, t restic.BlobType, p *packer) error
}

type uploadTask struct {
	packer *packer
	tpe    restic.BlobType
}

type packerUploader struct {
	uploadQueue chan uploadTask
}

func newPackerUploader(ctx context.Context, wg *errgroup.Group, repo savePacker, connections uint) *packerUploader {
	pu := &packerUploader{
		uploadQueue: make(chan uploadTask),
	}

	for i := 0; i < int(connections); i++ {
		wg.Go(func() error {
			for {
				select {
				case t, ok := <-pu.uploadQueue:
					if !ok {
						return nil
					}
					err := repo.savePacker(ctx, t.tpe, t.packer)
					if err != nil {
						return err
					}
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		})
	}

	return pu
}

func (pu *packerUploader) QueuePacker(ctx context.Context, t restic.BlobType, p *packer) (err error) {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case pu.uploadQueue <- uploadTask{tpe: t, packer: p}:
	}

	return nil
}

func (pu *packerUploader) TriggerShutdown() {
	close(pu.uploadQueue)
}
