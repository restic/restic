//go:build darwin || linux

package fs

import "github.com/restic/restic/internal/restic"

func (p *fdMetadataHandle) Xattr(ignoreListError bool) ([]restic.ExtendedAttribute, error) {
	// FIXME
	return xattrFromPath(p.Name(), ignoreListError)
}

func (p *fdMetadataHandle) SecurityDescriptor() (*[]byte, error) {
	return nil, nil
}
