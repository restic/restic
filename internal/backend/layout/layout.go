package layout

import (
	"github.com/restic/restic/internal/backend"
)

// Layout computes paths for file name storage.
type Layout interface {
	Filename(backend.Handle) string
	Dirname(backend.Handle) string
	Basedir(backend.FileType) (dir string, subdirs bool)
	Paths() []string
	Name() string
}
