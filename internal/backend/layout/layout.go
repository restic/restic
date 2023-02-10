package layout

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
)

// Layout computes paths for file name storage.
type Layout interface {
	Filename(restic.Handle) string
	Dirname(restic.Handle) string
	Basedir(restic.FileType) (dir string, subdirs bool)
	Paths() []string
	Name() string
}

// Filesystem is the abstraction of a file system used for a backend.
type Filesystem interface {
	Join(...string) string
	ReadDir(context.Context, string) ([]os.FileInfo, error)
	IsNotExist(error) bool
}

// ensure statically that *LocalFilesystem implements Filesystem.
var _ Filesystem = &LocalFilesystem{}

// LocalFilesystem implements Filesystem in a local path.
type LocalFilesystem struct {
}

// ReadDir returns all entries of a directory.
func (l *LocalFilesystem) ReadDir(ctx context.Context, dir string) ([]os.FileInfo, error) {
	f, err := fs.Open(dir)
	if err != nil {
		return nil, err
	}

	entries, err := f.Readdir(-1)
	if err != nil {
		return nil, errors.Wrap(err, "Readdir")
	}

	err = f.Close()
	if err != nil {
		return nil, errors.Wrap(err, "Close")
	}

	return entries, nil
}

// Join combines several path components to one.
func (l *LocalFilesystem) Join(paths ...string) string {
	return filepath.Join(paths...)
}

// IsNotExist returns true for errors that are caused by not existing files.
func (l *LocalFilesystem) IsNotExist(err error) bool {
	return os.IsNotExist(err)
}

var backendFilenameLength = len(restic.ID{}) * 2
var backendFilename = regexp.MustCompile(fmt.Sprintf("^[a-fA-F0-9]{%d}$", backendFilenameLength))

func hasBackendFile(ctx context.Context, fs Filesystem, dir string) (bool, error) {
	entries, err := fs.ReadDir(ctx, dir)
	if err != nil && fs.IsNotExist(err) {
		return false, nil
	}

	if err != nil {
		return false, errors.Wrap(err, "ReadDir")
	}

	for _, e := range entries {
		if backendFilename.MatchString(e.Name()) {
			return true, nil
		}
	}

	return false, nil
}

// ErrLayoutDetectionFailed is returned by DetectLayout() when the layout
// cannot be detected automatically.
var ErrLayoutDetectionFailed = errors.New("auto-detecting the filesystem layout failed")

// DetectLayout tries to find out which layout is used in a local (or sftp)
// filesystem at the given path. If repo is nil, an instance of LocalFilesystem
// is used.
func DetectLayout(ctx context.Context, repo Filesystem, dir string) (Layout, error) {
	debug.Log("detect layout at %v", dir)
	if repo == nil {
		repo = &LocalFilesystem{}
	}

	// key file in the "keys" dir (DefaultLayout)
	foundKeysFile, err := hasBackendFile(ctx, repo, repo.Join(dir, defaultLayoutPaths[restic.KeyFile]))
	if err != nil {
		return nil, err
	}

	// key file in the "key" dir (S3LegacyLayout)
	foundKeyFile, err := hasBackendFile(ctx, repo, repo.Join(dir, s3LayoutPaths[restic.KeyFile]))
	if err != nil {
		return nil, err
	}

	if foundKeysFile && !foundKeyFile {
		debug.Log("found default layout at %v", dir)
		return &DefaultLayout{
			Path: dir,
			Join: repo.Join,
		}, nil
	}

	if foundKeyFile && !foundKeysFile {
		debug.Log("found s3 layout at %v", dir)
		return &S3LegacyLayout{
			Path: dir,
			Join: repo.Join,
		}, nil
	}

	debug.Log("layout detection failed")
	return nil, ErrLayoutDetectionFailed
}

// ParseLayout parses the config string and returns a Layout. When layout is
// the empty string, DetectLayout is used. If that fails, defaultLayout is used.
func ParseLayout(ctx context.Context, repo Filesystem, layout, defaultLayout, path string) (l Layout, err error) {
	debug.Log("parse layout string %q for backend at %v", layout, path)
	switch layout {
	case "default":
		l = &DefaultLayout{
			Path: path,
			Join: repo.Join,
		}
	case "s3legacy":
		l = &S3LegacyLayout{
			Path: path,
			Join: repo.Join,
		}
	case "":
		l, err = DetectLayout(ctx, repo, path)

		// use the default layout if auto detection failed
		if errors.Is(err, ErrLayoutDetectionFailed) && defaultLayout != "" {
			debug.Log("error: %v, use default layout %v", err, defaultLayout)
			return ParseLayout(ctx, repo, defaultLayout, "", path)
		}

		if err != nil {
			return nil, err
		}
		debug.Log("layout detected: %v", l)
	default:
		return nil, errors.Errorf("unknown backend layout string %q, may be one of: default, s3legacy", layout)
	}

	return l, nil
}
