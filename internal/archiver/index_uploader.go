package archiver

import (
	"context"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

// IndexUploader polls the repo for full indexes and uploads them.
type IndexUploader struct {
	restic.Repository

	// Start is called when an index is to be uploaded.
	Start func()

	// Complete is called when uploading an index has finished.
	Complete func(id restic.ID)
}

// Upload periodically uploads full indexes to the repo. When shutdown is
// cancelled, the last index upload will finish and then Upload returns.
func (u IndexUploader) Upload(ctx, shutdown context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-shutdown.Done():
			return nil
		case <-ticker.C:
			full := u.Repository.Index().(*repository.MasterIndex).FullIndexes()
			for _, idx := range full {
				if u.Start != nil {
					u.Start()
				}

				id, err := repository.SaveIndex(ctx, u.Repository, idx)
				if err != nil {
					debug.Log("save indexes returned an error: %v", err)
					return err
				}
				if u.Complete != nil {
					u.Complete(id)
				}
			}
		}
	}
}
