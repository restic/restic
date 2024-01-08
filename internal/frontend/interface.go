package frontend

import (
	"github.com/restic/restic/internal/restic"
)

type Frontend interface {
	//OpenFile(name string, flag int, perm os.FileMode) (restic.FileContent, error)
	Prepare(elem ...string) []restic.LazyFileMetadata
}
