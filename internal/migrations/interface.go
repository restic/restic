package migrations

import (
	"context"

	"github.com/restic/restic/internal/restic"
)

// Migration implements a data migration.
type Migration interface {
	// Check returns true if the migration can be applied to a repo. If the option is not applicable it can return a specific reason.
	Check(context.Context, restic.Repository) (bool, string, error)

	RepoCheck() bool

	// Apply runs the migration.
	Apply(context.Context, restic.Repository) error

	// Name returns a short name.
	Name() string

	// Descr returns a description what the migration does.
	Desc() string
}
