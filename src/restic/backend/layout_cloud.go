package backend

import "restic"

// CloudLayout implements the default layout for cloud storage backends, as
// described in the Design document.
type CloudLayout struct {
	Path string
	Join func(...string) string
}

var cloudLayoutPaths = defaultLayoutPaths

// Dirname returns the directory path for a given file type and name.
func (l *CloudLayout) Dirname(h restic.Handle) string {
	return l.Join(l.Path, cloudLayoutPaths[h.Type])
}

// Filename returns a path to a file, including its name.
func (l *CloudLayout) Filename(h restic.Handle) string {
	name := h.Name

	if h.Type == restic.ConfigFile {
		name = "config"
	}

	return l.Join(l.Dirname(h), name)
}

// Paths returns all directory names
func (l *CloudLayout) Paths() (dirs []string) {
	for _, p := range cloudLayoutPaths {
		dirs = append(dirs, l.Join(l.Path, p))
	}
	return dirs
}

// Basedir returns the base dir name for files of type t.
func (l *CloudLayout) Basedir(t restic.FileType) string {
	return l.Join(l.Path, cloudLayoutPaths[t])
}
