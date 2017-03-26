package backend

import "restic"

// S3Layout implements the old layout used for s3 cloud storage backends, as
// described in the Design document.
type S3Layout struct {
	Path string
	Join func(...string) string
}

var s3LayoutPaths = map[restic.FileType]string{
	restic.DataFile:     "data",
	restic.SnapshotFile: "snapshot",
	restic.IndexFile:    "index",
	restic.LockFile:     "lock",
	restic.KeyFile:      "key",
}

// Dirname returns the directory path for a given file type and name.
func (l *S3Layout) Dirname(h restic.Handle) string {
	return l.Join(l.Path, s3LayoutPaths[h.Type])
}

// Filename returns a path to a file, including its name.
func (l *S3Layout) Filename(h restic.Handle) string {
	name := h.Name

	if h.Type == restic.ConfigFile {
		name = "config"
	}

	return l.Join(l.Dirname(h), name)
}

// Paths returns all directory names
func (l *S3Layout) Paths() (dirs []string) {
	for _, p := range s3LayoutPaths {
		dirs = append(dirs, l.Join(l.Path, p))
	}
	return dirs
}
