package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"restic"
	"restic/debug"
	"restic/errors"
	"restic/fs"
)

// Layout computes paths for file name storage.
type Layout interface {
	Filename(restic.Handle) string
	Dirname(restic.Handle) string
	Paths() []string
}

// Filesystem is the abstraction of a file system used for a backend.
type Filesystem interface {
	Join(...string) string
	ReadDir(string) ([]os.FileInfo, error)
}

// ensure statically that *LocalFilesystem implements Filesystem.
var _ Filesystem = &LocalFilesystem{}

// LocalFilesystem implements Filesystem in a local path.
type LocalFilesystem struct {
}

// ReadDir returns all entries of a directory.
func (l *LocalFilesystem) ReadDir(dir string) ([]os.FileInfo, error) {
	f, err := fs.Open(dir)
	if err != nil {
		return nil, err
	}

	entries, err := f.Readdir(-1)
	if err != nil {
		return nil, err
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// Join combines several path components to one.
func (l *LocalFilesystem) Join(paths ...string) string {
	return filepath.Join(paths...)
}

var backendFilenameLength = len(restic.ID{}) * 2
var backendFilename = regexp.MustCompile(fmt.Sprintf("^[a-fA-F0-9]{%d}$", backendFilenameLength))

func hasBackendFile(fs Filesystem, dir string) (bool, error) {
	entries, err := fs.ReadDir(dir)
	if err != nil && os.IsNotExist(errors.Cause(err)) {
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

var dataSubdirName = regexp.MustCompile("^[a-fA-F0-9]{2}$")

func hasSubdirBackendFile(fs Filesystem, dir string) (bool, error) {
	entries, err := fs.ReadDir(dir)
	if err != nil && os.IsNotExist(errors.Cause(err)) {
		return false, nil
	}

	if err != nil {
		return false, errors.Wrap(err, "ReadDir")
	}

	for _, subdir := range entries {
		if !dataSubdirName.MatchString(subdir.Name()) {
			continue
		}

		present, err := hasBackendFile(fs, fs.Join(dir, subdir.Name()))
		if err != nil {
			return false, err
		}

		if present {
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
func DetectLayout(repo Filesystem, dir string) (Layout, error) {
	if repo == nil {
		repo = &LocalFilesystem{}
	}

	// key file in the "keys" dir (DefaultLayout or CloudLayout)
	foundKeysFile, err := hasBackendFile(repo, repo.Join(dir, defaultLayoutPaths[restic.KeyFile]))
	if err != nil {
		return nil, err
	}

	// key file in the "key" dir (S3Layout)
	foundKeyFile, err := hasBackendFile(repo, repo.Join(dir, s3LayoutPaths[restic.KeyFile]))
	if err != nil {
		return nil, err
	}

	// data file in "data" directory (S3Layout or CloudLayout)
	foundDataFile, err := hasBackendFile(repo, repo.Join(dir, s3LayoutPaths[restic.DataFile]))
	if err != nil {
		return nil, err
	}

	// data file in subdir of "data" directory (DefaultLayout)
	foundDataSubdirFile, err := hasSubdirBackendFile(repo, repo.Join(dir, s3LayoutPaths[restic.DataFile]))
	if err != nil {
		return nil, err
	}

	if foundKeysFile && foundDataFile && !foundKeyFile && !foundDataSubdirFile {
		debug.Log("found cloud layout at %v", dir)
		return &CloudLayout{
			Path: dir,
			Join: repo.Join,
		}, nil
	}

	if foundKeysFile && foundDataSubdirFile && !foundKeyFile && !foundDataFile {
		debug.Log("found default layout at %v", dir)
		return &DefaultLayout{
			Path: dir,
			Join: repo.Join,
		}, nil
	}

	if foundKeyFile && foundDataFile && !foundKeysFile && !foundDataSubdirFile {
		debug.Log("found s3 layout at %v", dir)
		return &S3Layout{
			Path: dir,
			Join: repo.Join,
		}, nil
	}

	return nil, ErrLayoutDetectionFailed
}

// ParseLayout parses the config string and returns a Layout. When layout is
// the empty string, DetectLayout is used. If that fails, defaultLayout is used.
func ParseLayout(repo Filesystem, layout, defaultLayout, path string) (l Layout, err error) {
	debug.Log("parse layout string %q for backend at %v", layout, path)
	switch layout {
	case "default":
		l = &DefaultLayout{
			Path: path,
			Join: repo.Join,
		}
	case "cloud":
		l = &CloudLayout{
			Path: path,
			Join: repo.Join,
		}
	case "s3":
		l = &S3Layout{
			Path: path,
			Join: repo.Join,
		}
	case "":
		l, err = DetectLayout(repo, path)

		// use the default layout if auto detection failed
		if errors.Cause(err) == ErrLayoutDetectionFailed && defaultLayout != "" {
			return ParseLayout(repo, defaultLayout, "", path)
		}
	default:
		return nil, errors.Errorf("unknown backend layout string %q, may be one of default/cloud/s3", layout)
	}

	return l, nil
}
