//go:build aix || dragonfly || netbsd || openbsd
// +build aix dragonfly netbsd openbsd

package fs

import (
	"github.com/restic/restic/internal/restic"
)

// nodeRestoreExtendedAttributes is a no-op
func nodeRestoreExtendedAttributes(_ *restic.Node, _ string) error {
	return nil
}

// nodeFillExtendedAttributes is a no-op
func nodeFillExtendedAttributes(_ *restic.Node, _ string, _ bool) error {
	return nil
}
