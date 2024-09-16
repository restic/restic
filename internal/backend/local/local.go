package local

import (
	"context"
	"hash"
	"io"
	"os"
	"path/filepath"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/layout"
	"github.com/restic/restic/internal/backend/limiter"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/util"
	"github.com/restic/restic/internal/debug"
)

// Local represents a backend in a local directory.
type Local struct {
	Config
	layout.Layout
	util.Modes
}

// Ensure statically that *Local implements backend.Backend.
var _ backend.Backend = &Local{}

// NewFactory returns a new factory for local backends.
func NewFactory() location.Factory {
	return location.NewLimitedBackendFactory("local", ParseConfig, location.NoPassword, limiter.WrapBackendConstructor(Create), limiter.WrapBackendConstructor(Open))
}

func open(cfg Config) (*Local, error) {
	l := layout.NewDefaultLayout(cfg.Path, filepath.Join)
	m := util.DeriveModesFromStat(l, os.Stat)
	return &Local{
		Config: cfg,
		Layout: l,
		Modes:  m,
	}, nil
}

// Open opens the local backend as specified by config.
func Open(_ context.Context, cfg Config) (*Local, error) {
	debug.Log("open local backend at %v", cfg.Path)
	return open(cfg)
}

// Create creates all the necessary files and directories for a new local
// backend at dir. Afterwards a new config blob should be created.
func Create(_ context.Context, cfg Config) (*Local, error) {
	debug.Log("create local backend at %v", cfg.Path)
	be, err := open(cfg)
	if err != nil {
		return nil, err
	}
	err = util.Create(be.Filename(backend.Handle{Type: backend.ConfigFile}), be.Modes.Dir, be.Paths(), os.Lstat, os.MkdirAll)
	if err != nil {
		return nil, err
	}
	return be, nil
}

// Connections returns the number of configured connections.
func (b *Local) Connections() uint {
	return b.Config.Connections
}

// Hasher returns a hash function for calculating a content hash for the backend.
func (b *Local) Hasher() hash.Hash {
	return nil
}

// HasAtomicReplace returns whether Save() can atomically replace files.
func (b *Local) HasAtomicReplace() bool {
	return true
}

// IsNotExist returns true if the error is caused by a non-existing file.
func (b *Local) IsNotExist(err error) bool {
	return util.IsNotExist(err)
}

// IsPermanentError returns true if the error is permanent.
func (b *Local) IsPermanentError(err error) bool {
	return util.IsPermanentError(err)
}

// Save stores data in the backend at the handle.
func (b *Local) Save(_ context.Context, h backend.Handle, rd backend.RewindReader) error {
	fileName := b.Filename(h)
	// Create new file with a temporary name.
	tmpFilename := filepath.Base(fileName) + "-tmp-"

	saveOptions := util.SaveOptions{
		OpenTempFile: func(dir, name string) (util.File, error) {
			return tempFile(dir, name)
		},
		MkDir: func(dir string) error {
			return os.MkdirAll(dir, b.Modes.Dir)
		},
		Remove:      os.Remove,
		IsMacENOTTY: isMacENOTTY,
		Rename:      os.Rename,
		FsyncDir:    fsyncDir,
		SetFileReadonly: func(name string) error {
			return setFileReadonly(name, b.Modes.File)
		},
		DirMode:  b.Modes.Dir,
		FileMode: b.Modes.File,
	}

	return util.SaveWithOptions(fileName, tmpFilename, rd, saveOptions)
}

var tempFile = os.CreateTemp // Overridden by test.

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (b *Local) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	return util.DefaultLoad(ctx, h, length, offset, b.openReader, fn)
}

func (b *Local) openReader(_ context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
	openFile := func(name string) (util.File, error) {
		return os.Open(name)
	}
	return util.OpenReader(openFile, b.Filename(h), length, offset)
}

// Stat returns information about a blob.
func (b *Local) Stat(_ context.Context, h backend.Handle) (backend.FileInfo, error) {
	return util.Stat(os.Stat, b.Filename(h), h.Name)
}

// Remove removes the blob with the given name and type.
func (b *Local) Remove(_ context.Context, h backend.Handle) error {
	return util.Remove(b.Filename(h), setFileReadonly, os.Remove)
}

// List runs fn for each file in the backend which has the type t. When an
// error occurs (or fn returns an error), List stops and returns it.
func (b *Local) List(ctx context.Context, t backend.FileType, fn func(backend.FileInfo) error) error {
	openFunc := func(name string) (util.File, error) {
		return os.Open(name)
	}
	basedir, subdirs := b.Basedir(t)
	return util.List(ctx, basedir, subdirs, openFunc, fn)
}

// Delete removes the repository and all files.
func (b *Local) Delete(_ context.Context) error {
	return os.RemoveAll(b.Path)
}

// Close closes all open files.
func (b *Local) Close() error {
	// This does not need to do anything, all open files are closed within the
	// same function.
	return nil
}
