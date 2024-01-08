package frontend

import (
	"os"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

type LocalRootDirectory struct {
	frontend *LocalFrontend
	path     string
}

// statically ensure that LocalRootDirectory implements RootDirectory.
var _ restic.RootDirectory = &LocalRootDirectory{}

func (lrd *LocalRootDirectory) Path() string {
	return lrd.path
}

func (lrd *LocalRootDirectory) Join(name string) restic.RootDirectory {
	return &LocalRootDirectory{
		frontend: lrd.frontend,
		path:     lrd.frontend.FS.Join(lrd.path, name),
	}
}

func (lrd *LocalRootDirectory) StatDir() (restic.FileMetadata, error) {
	fm, err := lrd.frontend.stat(lrd.path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	tpe := fm.fi.Mode() & (os.ModeType | os.ModeCharDevice)
	if tpe != os.ModeDir {
		return fm, errors.Errorf("path is not a directory: %v", lrd.path)
	}

	return fm, nil
}

func (lrd *LocalRootDirectory) Equal(other restic.RootDirectory) bool {
	return lrd.path == other.Path()
}
