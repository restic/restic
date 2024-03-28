package frontend

import (
	"net/http"

	"github.com/restic/restic/internal/restic"
	"google.golang.org/api/drive/v3"
)

type GoogleDriveFileContent struct {
	drive *drive.Service
	file  *drive.File
	media *http.Response
	path  string
}

// staticaly ensure that GoogleDriveFileMetadata implements FileContent.
var _ restic.FileContent = &GoogleDriveFileContent{}

func (fc *GoogleDriveFileContent) Close() error {
	if fc.media != nil {
		return fc.media.Body.Close()
	}
	return nil
}

func (fc *GoogleDriveFileContent) Metadata() (restic.FileMetadata, error) {
	return &GoogleDriveFileMetadata{
		drive: fc.drive,
		file:  fc.file,
		path:  fc.path,
	}, nil
}

func (fc *GoogleDriveFileContent) Read(p []byte) (n int, err error) {
	if fc.media == nil {
		fc.media, err = fc.drive.Files.Get(fc.file.Id).Download()
		if err != nil {
			return 0, err
		}
	}
	return fc.media.Body.Read(p)
}
