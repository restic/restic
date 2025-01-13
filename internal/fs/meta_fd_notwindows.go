//go:build darwin || linux

package fs

import "github.com/restic/restic/internal/restic"

func (p *fdMetadataHandle) Xattr(ignoreListError bool) ([]restic.ExtendedAttribute, error) {
	path := p.Name()
	return xattrFromPath(
		path,
		func() ([]string, error) { return flistxattr(p.f) },
		func(attr string) ([]byte, error) { return fgetxattr(p.f, attr) },
		ignoreListError,
	)
}

func (p *fdMetadataHandle) SecurityDescriptor() (*[]byte, error) {
	return nil, nil
}
