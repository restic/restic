package frontend

import (
	"github.com/restic/restic/internal/restic"
	"google.golang.org/api/drive/v3"
)

type GoogleDriveLazyFileMetadata struct {
	drive    *drive.Service
	delegate *GoogleDriveFileMetadata
	name     string
	path     string
	id       string
}

// statically ensure that GoogleDriveFileMetadata implements FileMetadata.
var _ restic.LazyFileMetadata = &GoogleDriveLazyFileMetadata{}

func (lfm *GoogleDriveLazyFileMetadata) Equal(other restic.FileMetadata) bool {
	if other == nil {
		return false
	}
	return lfm.path == other.Path()
}

func (lfm *GoogleDriveLazyFileMetadata) Init() error {
	if lfm.delegate == nil {
		file, err := lfm.drive.Files.Get(lfm.id).Do()
		if err != nil {
			return err
		}

		lfm.delegate = &GoogleDriveFileMetadata{
			drive: lfm.drive,
			file:  file,
			path:  lfm.path,
		}
	}
	return nil
}

func (fm *GoogleDriveLazyFileMetadata) Name() string {
	return fm.name
}

func (lfm *GoogleDriveLazyFileMetadata) Path() string {
	return lfm.path
}

func (lfm *GoogleDriveLazyFileMetadata) AbsPath() (string, error) {
	return lfm.path, nil
}

func (lfm *GoogleDriveLazyFileMetadata) Abs() (restic.LazyFileMetadata, error) {
	return lfm, nil
}

func (lfm *GoogleDriveLazyFileMetadata) Children() ([]restic.LazyFileMetadata, error) {
	return lfm.delegate.Children()
}

func (lfm *GoogleDriveLazyFileMetadata) ChildrenWithFlag(flags int) ([]restic.LazyFileMetadata, error) {
	return lfm.delegate.ChildrenWithFlag(flags)
}

func (lfm *GoogleDriveLazyFileMetadata) Clean() restic.LazyFileMetadata {
	return lfm
}

func (lfm *GoogleDriveLazyFileMetadata) Exist() bool {
	if lfm.delegate != nil {
		return true
	}
	err := lfm.Init()
	return err == nil
}

func (lfm *GoogleDriveLazyFileMetadata) FileChanged(node *restic.Node) bool {
	return lfm.delegate.FileChanged(node)
}

func (lfm *GoogleDriveLazyFileMetadata) Mode() restic.FileMode {
	return lfm.delegate.Mode()
}

func (lfm *GoogleDriveLazyFileMetadata) Node(snPath string, withAtime bool) (*restic.Node, error) {
	return lfm.delegate.Node(snPath, withAtime)
}

func (lfm *GoogleDriveLazyFileMetadata) Size() int64 {
	return lfm.delegate.Size()
}

func (lfm *GoogleDriveLazyFileMetadata) OpenFile() (restic.FileContent, error) {
	lfm.Init()
	return &GoogleDriveFileContent{
		drive: lfm.drive,
		file:  lfm.delegate.file,
		path:  lfm.path,
	}, nil
}

func (lfm *GoogleDriveLazyFileMetadata) PathComponents(includeRelative bool) (components []string, virtualPrefix bool) {
	panic("unimplemented")
}

func (lfm *GoogleDriveLazyFileMetadata) RootDirectory() restic.RootDirectory {
	panic("unimplemented")
}

func (lfm *GoogleDriveLazyFileMetadata) DeviceID() (deviceID uint64, err error) {
	return lfm.delegate.DeviceID()
}
