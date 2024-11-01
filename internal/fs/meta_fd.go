//go:build darwin || linux || windows

package fs

import "os"

type fdMetadataHandle struct {
	name string
	f    *os.File
}

var _ metadataHandle = &fdMetadataHandle{}

func newFdMetadataHandle(name string, f *os.File) *fdMetadataHandle {
	return &fdMetadataHandle{
		name: name,
		f:    f,
	}
}

func (p *fdMetadataHandle) Name() string {
	return p.name
}

func (p *fdMetadataHandle) Stat() (*ExtendedFileInfo, error) {
	fi, err := p.f.Stat()
	if err != nil {
		return nil, err
	}
	return extendedStat(fi), nil
}

func (p *fdMetadataHandle) Readlink() (string, error) {
	return Freadlink(p.f.Fd(), fixpath(p.name))
}
