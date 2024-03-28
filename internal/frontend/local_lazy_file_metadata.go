package frontend

import (
	"errors"
	"os"

	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

type LocalLazyFileMetadata struct {
	frontend *LocalFrontend
	delegate restic.FileMetadata
	name     string
	path     string
}

// statically ensure that GoogleDriveFileMetadata implements FileMetadata.
var _ restic.LazyFileMetadata = &LocalLazyFileMetadata{}

func (lfm *LocalLazyFileMetadata) Equal(other restic.FileMetadata) bool {
	if other == nil {
		return false
	}
	return lfm.path == other.Path()
}

func (lfm *LocalLazyFileMetadata) Exist() bool {
	_, err := lfm.frontend.FS.Lstat(lfm.path)
	return !errors.Is(err, os.ErrNotExist)
}

func (lfm *LocalLazyFileMetadata) OpenFile() (restic.FileContent, error) {
	return lfm.frontend.openFile(lfm.path, fs.O_RDONLY|fs.O_NOFOLLOW, 0)
}

func (lfm *LocalLazyFileMetadata) RootDirectory() restic.RootDirectory {
	return &LocalRootDirectory{
		frontend: lfm.frontend,
		path:     rootDirectory(lfm.frontend.FS, lfm.path),
	}
}

func (lfm *LocalLazyFileMetadata) PathComponents(includeRelative bool) (components []string, virtualPrefix bool) {
	return pathComponents(lfm.frontend.FS, lfm.path, includeRelative)
}

func (lfm *LocalLazyFileMetadata) Abs() (restic.LazyFileMetadata, error) {
	absPath, err := lfm.AbsPath()
	return &LocalLazyFileMetadata{
		frontend: lfm.frontend,
		name:     lfm.name,
		path:     absPath,
	}, err
}

func (lfm *LocalLazyFileMetadata) Clean() restic.LazyFileMetadata {

	return &LocalLazyFileMetadata{
		frontend: lfm.frontend,
		name:     lfm.name,
		path:     lfm.frontend.FS.Clean(lfm.path),
	}
}

func (lfm *LocalLazyFileMetadata) Name() string {
	return lfm.name
}

func (lfm *LocalLazyFileMetadata) Path() string {
	return lfm.path
}

func (lfm *LocalLazyFileMetadata) AbsPath() (string, error) {
	return lfm.frontend.FS.Abs(lfm.path)
}

func (lfm *LocalLazyFileMetadata) Init() error {
	if lfm.delegate == nil {
		fm, err := lfm.frontend.lstat(lfm.path)
		if err != nil {
			return err
		}
		lfm.delegate = fm
	}
	return nil
}

func (lfm *LocalLazyFileMetadata) Children() ([]restic.LazyFileMetadata, error) {
	lfm.Init()
	return lfm.delegate.Children()
}

func (lfm *LocalLazyFileMetadata) ChildrenWithFlag(flag int) ([]restic.LazyFileMetadata, error) {
	lfm.Init()
	return lfm.delegate.ChildrenWithFlag(flag)
}

func (lfm *LocalLazyFileMetadata) FileChanged(node *restic.Node) bool {
	lfm.Init()
	return lfm.delegate.FileChanged(node)
}

func (lfm *LocalLazyFileMetadata) Mode() restic.FileMode {
	lfm.Init()
	return lfm.delegate.Mode()
}

func (lfm *LocalLazyFileMetadata) Node(snPath string, withAtime bool) (*restic.Node, error) {
	lfm.Init()
	return lfm.delegate.Node(snPath, withAtime)
}

func (lfm *LocalLazyFileMetadata) Size() int64 {
	lfm.Init()
	return lfm.delegate.Size()
}

func (lfm *LocalLazyFileMetadata) DeviceID() (deviceID uint64, err error) {
	lfm.Init()
	return lfm.delegate.DeviceID()
}
