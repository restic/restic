package layout

import (
	"encoding/hex"

	"github.com/restic/restic/internal/backend"
)

// DefaultLayout implements the default layout for local and sftp backends, as
// described in the Design document. The `data` directory has one level of
// subdirs, two characters each (taken from the first two characters of the
// file name).
type DefaultLayout struct {
	path string
	join func(...string) string
}

var defaultLayoutPaths = map[backend.FileType]string{
	backend.PackFile:     "data",
	backend.SnapshotFile: "snapshots",
	backend.IndexFile:    "index",
	backend.LockFile:     "locks",
	backend.KeyFile:      "keys",
}

func NewDefaultLayout(path string, join func(...string) string) *DefaultLayout {
	return &DefaultLayout{
		path: path,
		join: join,
	}
}

func (l *DefaultLayout) String() string {
	return "<DefaultLayout>"
}

// Name returns the name for this layout.
func (l *DefaultLayout) Name() string {
	return "default"
}

// Dirname returns the directory path for a given file type and name.
func (l *DefaultLayout) Dirname(h backend.Handle) string {
	p := defaultLayoutPaths[h.Type]

	if h.Type == backend.PackFile && len(h.Name) > 2 {
		p = l.join(p, h.Name[:2]) + "/"
	}

	return l.join(l.path, p) + "/"
}

// Filename returns a path to a file, including its name.
func (l *DefaultLayout) Filename(h backend.Handle) string {
	name := h.Name
	if h.Type == backend.ConfigFile {
		return l.join(l.path, "config")
	}

	return l.join(l.Dirname(h), name)
}

// Paths returns all directory names needed for a repo.
func (l *DefaultLayout) Paths() (dirs []string) {
	for _, p := range defaultLayoutPaths {
		dirs = append(dirs, l.join(l.path, p))
	}

	// also add subdirs
	for i := 0; i < 256; i++ {
		subdir := hex.EncodeToString([]byte{byte(i)})
		dirs = append(dirs, l.join(l.path, defaultLayoutPaths[backend.PackFile], subdir))
	}

	return dirs
}

// Basedir returns the base dir name for type t.
func (l *DefaultLayout) Basedir(t backend.FileType) (dirname string, subdirs bool) {
	if t == backend.PackFile {
		subdirs = true
	}

	dirname = l.join(l.path, defaultLayoutPaths[t])
	return
}
