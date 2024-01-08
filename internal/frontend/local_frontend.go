package frontend

import (
	"os"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

// Flags for the ChangeIgnoreFlags bitfield.
const (
	ChangeIgnoreCtime = 1 << iota
	ChangeIgnoreInode
)

type LocalFrontend struct {
	FS fs.FS
	// Flags controlling change detection. See doc/040_backup.rst for details.
	ChangeIgnoreFlags uint
}

// statically ensure that Local implements Frontend.
var _ Frontend = &LocalFrontend{}

func (frontend *LocalFrontend) Prepare(paths ...string) []restic.LazyFileMetadata {
	result := make([]restic.LazyFileMetadata, len(paths))
	for i, path := range paths {
		result[i] = &LocalLazyFileMetadata{
			frontend: frontend,
			name:     frontend.FS.Base(path),
			path:     path,
		}
	}
	return result
}

func (frontend *LocalFrontend) lstat(path string) (restic.FileMetadata, error) {
	fi, err := frontend.FS.Lstat(path)
	if err != nil {
		return nil, err
	}
	return &LocalFileMetadata{
		frontend: frontend,
		fi:       fi,
		path:     path,
	}, nil
}

func (frontend *LocalFrontend) openFile(name string, flag int, perm os.FileMode) (restic.FileContent, error) {
	file, err := frontend.FS.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &LocalFileContent{
		frontend: frontend,
		file:     file,
		path:     name,
	}, nil
}

func (frontend *LocalFrontend) stat(path string) (*LocalFileMetadata, error) {
	fi, err := frontend.FS.Stat(path)
	if err != nil {
		return nil, err
	}
	return &LocalFileMetadata{
		frontend: frontend,
		fi:       fi,
		path:     path,
	}, nil
}

// flags are passed to fs.OpenFile. O_RDONLY is implied.
func (frontend *LocalFrontend) readdirnames(dir string, flags int) ([]string, error) {
	f, err := frontend.FS.OpenFile(dir, fs.O_RDONLY|flags, 0)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	entries, err := f.Readdirnames(-1)
	if err != nil {
		_ = f.Close()
		return nil, errors.Wrapf(err, "Readdirnames %v failed", dir)
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// pathComponents returns all path components of p. If a virtual directory
// (volume name on Windows) is added, virtualPrefix is set to true. See the
// tests for examples.
func pathComponents(fs fs.FS, p string, includeRelative bool) (components []string, virtualPrefix bool) {
	volume := fs.VolumeName(p)

	if !fs.IsAbs(p) {
		if !includeRelative {
			p = fs.Join(fs.Separator(), p)
		}
	}

	p = fs.Clean(p)

	for {
		dir, file := fs.Dir(p), fs.Base(p)

		if p == dir {
			break
		}

		components = append(components, file)
		p = dir
	}

	// reverse components
	for i := len(components)/2 - 1; i >= 0; i-- {
		opp := len(components) - 1 - i
		components[i], components[opp] = components[opp], components[i]
	}

	if volume != "" {
		// strip colon
		if len(volume) == 2 && volume[1] == ':' {
			volume = volume[:1]
		}

		components = append([]string{volume}, components...)
		virtualPrefix = true
	}

	return components, virtualPrefix
}

// rootDirectory returns the directory which contains the first element of target.
func rootDirectory(f fs.FS, target string) string {
	if target == "" {
		return ""
	}

	if f.IsAbs(target) {
		return f.Join(f.VolumeName(target), f.Separator())
	}

	target = f.Clean(target)
	pc, _ := pathComponents(f, target, true)

	rel := "."
	for _, c := range pc {
		if c == ".." {
			rel = f.Join(rel, c)
		}
	}

	return rel
}
