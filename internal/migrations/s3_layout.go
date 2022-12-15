package migrations

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/restic/restic/internal/backend/layout"
	"github.com/restic/restic/internal/backend/s3"
	"github.com/restic/restic/internal/cache"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

func init() {
	register(&S3Layout{})
}

// S3Layout migrates a repository on an S3 backend from the "s3legacy" to the
// "default" layout.
type S3Layout struct{}

func toS3Backend(repo restic.Repository) *s3.Backend {
	b := repo.Backend()
	// unwrap cache
	if be, ok := b.(*cache.Backend); ok {
		b = be.Backend
	}

	be, ok := b.(*s3.Backend)
	if !ok {
		debug.Log("backend is not s3")
		return nil
	}
	return be
}

// Check tests whether the migration can be applied.
func (m *S3Layout) Check(ctx context.Context, repo restic.Repository) (bool, string, error) {
	be := toS3Backend(repo)
	if be == nil {
		debug.Log("backend is not s3")
		return false, "backend is not s3", nil
	}

	if be.Layout.Name() != "s3legacy" {
		debug.Log("layout is not s3legacy")
		return false, "not using the legacy s3 layout", nil
	}

	return true, "", nil
}

func (m *S3Layout) RepoCheck() bool {
	return false
}

func retry(max int, fail func(err error), f func() error) error {
	var err error
	for i := 0; i < max; i++ {
		err = f()
		if err == nil {
			return nil
		}
		if fail != nil {
			fail(err)
		}
	}
	return err
}

// maxErrors for retrying renames on s3.
const maxErrors = 20

func (m *S3Layout) moveFiles(ctx context.Context, be *s3.Backend, l layout.Layout, t restic.FileType) error {
	printErr := func(err error) {
		fmt.Fprintf(os.Stderr, "renaming file returned error: %v\n", err)
	}

	return be.List(ctx, t, func(fi restic.FileInfo) error {
		h := restic.Handle{Type: t, Name: fi.Name}
		debug.Log("move %v", h)

		return retry(maxErrors, printErr, func() error {
			return be.Rename(ctx, h, l)
		})
	})
}

// Apply runs the migration.
func (m *S3Layout) Apply(ctx context.Context, repo restic.Repository) error {
	be := toS3Backend(repo)
	if be == nil {
		debug.Log("backend is not s3")
		return errors.New("backend is not s3")
	}

	oldLayout := &layout.S3LegacyLayout{
		Path: be.Path(),
		Join: path.Join,
	}

	newLayout := &layout.DefaultLayout{
		Path: be.Path(),
		Join: path.Join,
	}

	be.Layout = oldLayout

	for _, t := range []restic.FileType{
		restic.SnapshotFile,
		restic.PackFile,
		restic.KeyFile,
		restic.LockFile,
	} {
		err := m.moveFiles(ctx, be, newLayout, t)
		if err != nil {
			return err
		}
	}

	be.Layout = newLayout

	return nil
}

// Name returns the name for this migration.
func (m *S3Layout) Name() string {
	return "s3_layout"
}

// Desc returns a short description what the migration does.
func (m *S3Layout) Desc() string {
	return "move files from 's3legacy' to the 'default' repository layout"
}
