package restic

import (
	"os"

	"github.com/restic/restic/internal/archiver"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

func ResticDir(d string, n string) (string, error) {
	return os.MkdirTemp(d, n)
}

func Archiver(repo restic.Repository) *archiver.Archiver {
	return archiver.New(repo, fs.Track{FS: fs.Local{}}, archiver.Options{})
}
