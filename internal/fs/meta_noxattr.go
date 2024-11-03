//go:build aix || dragonfly || netbsd || openbsd
// +build aix dragonfly netbsd openbsd

package fs

import "github.com/restic/restic/internal/restic"

func (p *pathMetadataHandle) Xattr(ignoreListError bool) ([]restic.ExtendedAttribute, error) {
	return nil, nil
}
