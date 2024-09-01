package layout

import (
	"path"

	"github.com/restic/restic/internal/backend"
)

// RESTLayout implements the default layout for the REST protocol.
type RESTLayout struct {
	url string
}

var restLayoutPaths = defaultLayoutPaths

func NewRESTLayout(url string) *RESTLayout {
	return &RESTLayout{
		url: url,
	}
}

func (l *RESTLayout) String() string {
	return "<RESTLayout>"
}

// Name returns the name for this layout.
func (l *RESTLayout) Name() string {
	return "rest"
}

// Dirname returns the directory path for a given file type and name.
func (l *RESTLayout) Dirname(h backend.Handle) string {
	if h.Type == backend.ConfigFile {
		return l.url + "/"
	}

	return l.url + path.Join("/", restLayoutPaths[h.Type]) + "/"
}

// Filename returns a path to a file, including its name.
func (l *RESTLayout) Filename(h backend.Handle) string {
	name := h.Name

	if h.Type == backend.ConfigFile {
		name = "config"
	}

	return l.url + path.Join("/", restLayoutPaths[h.Type], name)
}

// Paths returns all directory names
func (l *RESTLayout) Paths() (dirs []string) {
	for _, p := range restLayoutPaths {
		dirs = append(dirs, l.url+path.Join("/", p))
	}
	return dirs
}

// Basedir returns the base dir name for files of type t.
func (l *RESTLayout) Basedir(t backend.FileType) (dirname string, subdirs bool) {
	return l.url + path.Join("/", restLayoutPaths[t]), false
}
