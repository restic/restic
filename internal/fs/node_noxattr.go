//go:build aix || dragonfly || openbsd
// +build aix dragonfly openbsd

package fs

import "github.com/restic/restic/internal/data"

// nodeRestoreExtendedAttributes is a no-op
func nodeRestoreExtendedAttributes(_ *data.Node, _ string, _ func(xattrName string) bool) error {
	return nil
}

// nodeFillExtendedAttributes is a no-op
func nodeFillExtendedAttributes(_ *data.Node, _ string, _ bool, _ func(format string, args ...any)) error {
	return nil
}
