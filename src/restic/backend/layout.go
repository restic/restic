package backend

import (
	"restic"
)

// Layout computes paths for file name storage.
type Layout interface {
	Filename(restic.Handle) string
	Dirname(restic.Handle) string
	Paths() []string
}
