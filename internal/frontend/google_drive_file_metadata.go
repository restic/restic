package frontend

import (
	"path/filepath"
	"time"

	"github.com/restic/restic/internal/restic"
	"google.golang.org/api/drive/v3"
)

type GoogleDriveFileMetadata struct {
	drive *drive.Service
	file  *drive.File
	path  string
}

// statically ensure that GoogleDriveFileMetadata implements FileMetadata.
var _ restic.FileMetadata = &GoogleDriveFileMetadata{}

// Equal implements restic.FileMetadata.
func (fm *GoogleDriveFileMetadata) Equal(other restic.FileMetadata) bool {
	if other == nil {
		return false
	}
	return fm.path == other.Path()
}

func (fm *GoogleDriveFileMetadata) Size() int64 {
	return fm.file.Size
}

func (fm *GoogleDriveFileMetadata) Children() ([]restic.LazyFileMetadata, error) {
	search, err := fm.drive.Files.List().Q("'" + fm.file.Id + "' in parents").Fields("files(id, name, mimeType, modifiedTime)").Do()
	if err != nil {
		return nil, err
	}
	result := make([]restic.LazyFileMetadata, len(search.Files))
	for i, child := range search.Files {
		path := filepath.Join(fm.path, child.Name)
		result[i] = &GoogleDriveLazyFileMetadata{
			drive: fm.drive,
			path:  path,
			id:    child.Id,
			delegate: &GoogleDriveFileMetadata{
				drive: fm.drive,
				file:  child,
				path:  path,
			},
		}
	}
	return result, nil
}

func (fm *GoogleDriveFileMetadata) ChildrenWithFlag(flags int) ([]restic.LazyFileMetadata, error) {
	return fm.Children()
}

func (fm *GoogleDriveFileMetadata) Path() string {
	return fm.path
}

func (fm *GoogleDriveFileMetadata) AbsPath() (string, error) {
	return fm.path, nil
}

func (fm *GoogleDriveFileMetadata) Node(snPath string, withAtime bool) (*restic.Node, error) {
	panic("unimplemented")
}

func (fm *GoogleDriveFileMetadata) FileChanged(node *restic.Node) bool {
	switch {
	case node == nil:
		return true
	case node.Type != "file":
		// We're only called for regular files, so this is a type change.
		return true
	case uint64(fm.Size()) != node.Size:
		return true
	case !fm.modTime().Equal(node.ModTime):
		return true
	}
	return false
}

func (fm *GoogleDriveFileMetadata) modTime() time.Time {
	modifiedTime, _ := time.Parse(time.RFC3339, fm.file.ModifiedTime)
	return modifiedTime
}

func (fm *GoogleDriveFileMetadata) Mode() restic.FileMode {
	switch fm.file.MimeType {
	case "application/vnd.google-apps.folder":
		return restic.DIRECTORY
	case "application/vnd.google-apps.document", "application/vnd.google-apps.presentation", "application/vnd.google-apps.spreadsheet":
		return restic.SOCKET // ignore for now
	default:
		return restic.REGULAR
	}
}

func (fm *GoogleDriveFileMetadata) Name() string {
	return fm.file.Name
}

func (*GoogleDriveFileMetadata) DeviceID() (deviceID uint64, err error) {
	return 0, nil
}
