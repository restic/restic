package backend

import "restic"

// DefaultLayout implements the default layout for local and sftp backends, as
// described in the Design document. The `data` directory has one level of
// subdirs, two characters each (taken from the first two characters of the
// file name).
type DefaultLayout struct {
	Path string
	Join func(...string) string
}

var defaultLayoutPaths = map[restic.FileType]string{
	restic.DataFile:     "data",
	restic.SnapshotFile: "snapshots",
	restic.IndexFile:    "index",
	restic.LockFile:     "locks",
	restic.KeyFile:      "keys",
}

// Dirname returns the directory path for a given file type and name.
func (l *DefaultLayout) Dirname(h restic.Handle) string {
	p := defaultLayoutPaths[h.Type]

	if h.Type == restic.DataFile && len(h.Name) > 2 {
		p = l.Join(p, h.Name[:2]) + "/"
	}

	return l.Join(l.Path, p) + "/"
}

// Filename returns a path to a file, including its name.
func (l *DefaultLayout) Filename(h restic.Handle) string {
	name := h.Name
	if h.Type == restic.ConfigFile {
		return l.Join(l.Path, "config")
	}

	return l.Join(l.Dirname(h), name)
}

// Paths returns all directory names
func (l *DefaultLayout) Paths() (dirs []string) {
	for _, p := range defaultLayoutPaths {
		dirs = append(dirs, l.Join(l.Path, p))
	}
	return dirs
}

// Basedir returns the base dir name for type t.
func (l *DefaultLayout) Basedir(t restic.FileType) string {
	return l.Join(l.Path, defaultLayoutPaths[t])
}
