//go:build !windows
// +build !windows

package fs

func (p *pathMetadataHandle) SecurityDescriptor() (*[]byte, error) {
	return nil, nil
}
