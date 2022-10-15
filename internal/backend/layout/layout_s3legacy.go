package layout

import "github.com/restic/restic/internal/restic"

// S3LegacyLayout implements the old layout used for s3 cloud storage backends, as
// described in the Design document.
type S3LegacyLayout struct {
	URL  string
	Path string
	Join func(...string) string
}

var s3LayoutPaths = map[restic.FileType]string{
	restic.PackFile:     "data",
	restic.SnapshotFile: "snapshot",
	restic.IndexFile:    "index",
	restic.LockFile:     "lock",
	restic.KeyFile:      "key",
}

func (l *S3LegacyLayout) String() string {
	return "<S3LegacyLayout>"
}

// Name returns the name for this layout.
func (l *S3LegacyLayout) Name() string {
	return "s3legacy"
}

// join calls Join with the first empty elements removed.
func (l *S3LegacyLayout) join(url string, items ...string) string {
	for len(items) > 0 && items[0] == "" {
		items = items[1:]
	}

	path := l.Join(items...)
	if path == "" || path[0] != '/' {
		if url != "" && url[len(url)-1] != '/' {
			url += "/"
		}
	}

	return url + path
}

// Dirname returns the directory path for a given file type and name.
func (l *S3LegacyLayout) Dirname(h restic.Handle) string {
	if h.Type == restic.ConfigFile {
		return l.URL + l.Join(l.Path, "/")
	}

	return l.join(l.URL, l.Path, s3LayoutPaths[h.Type]) + "/"
}

// Filename returns a path to a file, including its name.
func (l *S3LegacyLayout) Filename(h restic.Handle) string {
	name := h.Name

	if h.Type == restic.ConfigFile {
		name = "config"
	}

	return l.join(l.URL, l.Path, s3LayoutPaths[h.Type], name)
}

// Paths returns all directory names
func (l *S3LegacyLayout) Paths() (dirs []string) {
	for _, p := range s3LayoutPaths {
		dirs = append(dirs, l.Join(l.Path, p))
	}
	return dirs
}

// Basedir returns the base dir name for type t.
func (l *S3LegacyLayout) Basedir(t restic.FileType) (dirname string, subdirs bool) {
	return l.Join(l.Path, s3LayoutPaths[t]), false
}
