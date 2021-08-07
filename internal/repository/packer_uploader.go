package repository

import (
	"context"

	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

// SavePacker implements saving a pack in the repository.
type SavePacker interface {
	savePacker(ctx context.Context, t restic.BlobType, p *Packer) error
}

type uploadTask struct {
	packer *Packer
	tpe    restic.BlobType
}

type packerUploader struct {
	uploadQueue chan uploadTask
}

func newPackerUploader(ctx context.Context, wg *errgroup.Group, repo SavePacker, connections uint) *packerUploader {
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

func (pu *packerUploader) QueuePacker(ctx context.Context, t restic.BlobType, p *Packer) (err error) {
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
