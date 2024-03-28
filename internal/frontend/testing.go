package frontend

import (
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

func CreateTestLazyFileMetadata(path string) restic.LazyFileMetadata {
	f := &LocalFrontend{FS: fs.Local{}}
	return &LocalLazyFileMetadata{
		frontend: f,
		path:     path,
	}
}

func CreateTestRootDirectory(path string) restic.RootDirectory {
	f := &LocalFrontend{FS: fs.Local{}}
	return &LocalRootDirectory{
		frontend: f,
		path:     path,
	}
}

func NodeFromLocalFileMetadata(fm LocalFileMetadata) (*restic.Node, error) {
	return restic.NodeFromFileInfo(fm.path, fm.fi)
}
