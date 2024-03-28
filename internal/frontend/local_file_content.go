package frontend

import (
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

type LocalFileContent struct {
	frontend *LocalFrontend
	path     string
	file     fs.File
}

// statically ensure that LocalFileContent implements FileContent.
var _ restic.FileContent = &LocalFileContent{}

func (fc *LocalFileContent) Read(p []byte) (n int, err error) {
	return fc.file.Read(p)
}

func (fc *LocalFileContent) Close() error {
	return fc.file.Close()
}

func (fc *LocalFileContent) Metadata() (restic.FileMetadata, error) {
	fi, err := fc.file.Stat()
	if err != nil {
		return nil, err
	}
	return &LocalFileMetadata{
		frontend: fc.frontend,
		fi:       fi,
		path:     fc.path,
	}, nil
}
