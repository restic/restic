package local

import (
	"context"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/layout"
	"github.com/restic/restic/internal/backend/limiter"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/util"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"

	"github.com/cenkalti/backoff/v4"
)

// Local is a backend in a local directory.
type Local struct {
	Config
	layout.Layout
	util.Modes
}

// ensure statically that *Local implements backend.Backend.
var _ backend.Backend = &Local{}

var errTooShort = fmt.Errorf("file is too short")

func NewFactory() location.Factory {
	return location.NewLimitedBackendFactory("local", ParseConfig, location.NoPassword, limiter.WrapBackendConstructor(Create), limiter.WrapBackendConstructor(Open))
}

const defaultLayout = "default"

func open(ctx context.Context, cfg Config) (*Local, error) {
	l, err := layout.ParseLayout(ctx, &layout.LocalFilesystem{}, cfg.Layout, defaultLayout, cfg.Path)
	if err != nil {
		return nil, err
	}

	fi, err := fs.Stat(l.Filename(backend.Handle{Type: backend.ConfigFile}))
	m := util.DeriveModesFromFileInfo(fi, err)
	debug.Log("using (%03O file, %03O dir) permissions", m.File, m.Dir)

	return &Local{
		Config: cfg,
		Layout: l,
		Modes:  m,
	}, nil
}

// Open opens the local backend as specified by config.
func Open(ctx context.Context, cfg Config) (*Local, error) {
	debug.Log("open local backend at %v (layout %q)", cfg.Path, cfg.Layout)
	return open(ctx, cfg)
}

// Create creates all the necessary files and directories for a new local
// backend at dir. Afterwards a new config blob should be created.
func Create(ctx context.Context, cfg Config) (*Local, error) {
	debug.Log("create local backend at %v (layout %q)", cfg.Path, cfg.Layout)

	be, err := open(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// test if config file already exists
	_, err = fs.Lstat(be.Filename(backend.Handle{Type: backend.ConfigFile}))
	if err == nil {
		return nil, errors.New("config file already exists")
	}

	// create paths for data and refs
	for _, d := range be.Paths() {
		err := fs.MkdirAll(d, be.Modes.Dir)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return be, nil
}

func (b *Local) Connections() uint {
	return b.Config.Connections
}

// Hasher may return a hash function for calculating a content hash for the backend
func (b *Local) Hasher() hash.Hash {
	return nil
}

// HasAtomicReplace returns whether Save() can atomically replace files
func (b *Local) HasAtomicReplace() bool {
	return true
}

// IsNotExist returns true if the error is caused by a non existing file.
func (b *Local) IsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}

func (b *Local) IsPermanentError(err error) bool {
	return b.IsNotExist(err) || errors.Is(err, errTooShort) || errors.Is(err, os.ErrPermission)
}

// Save stores data in the backend at the handle.
func (b *Local) Save(_ context.Context, h backend.Handle, rd backend.RewindReader) (err error) {
	finalname := b.Filename(h)
	dir := filepath.Dir(finalname)

	defer func() {
		// Mark non-retriable errors as such
		if errors.Is(err, syscall.ENOSPC) || os.IsPermission(err) {
			err = backoff.Permanent(err)
		}
	}()

	// Create new file with a temporary name.
	tmpname := filepath.Base(finalname) + "-tmp-"
	f, err := tempFile(dir, tmpname)

	if b.IsNotExist(err) {
		debug.Log("error %v: creating dir", err)

		// error is caused by a missing directory, try to create it
		mkdirErr := fs.MkdirAll(dir, b.Modes.Dir)
		if mkdirErr != nil {
			debug.Log("error creating dir %v: %v", dir, mkdirErr)
		} else {
			// try again
			f, err = tempFile(dir, tmpname)
		}
	}

	if err != nil {
		return errors.WithStack(err)
	}

	defer func(f *os.File) {
		if err != nil {
			_ = f.Close() // Double Close is harmless.
			// Remove after Rename is harmless: we embed the final name in the
			// temporary's name and no other goroutine will get the same data to
			// Save, so the temporary name should never be reused by another
			// goroutine.
			_ = fs.Remove(f.Name())
		}
	}(f)

	// preallocate disk space
	if size := rd.Length(); size > 0 {
		if err := fs.PreallocateFile(f, size); err != nil {
			debug.Log("Failed to preallocate %v with size %v: %v", finalname, size, err)
		}
	}

	// save data, then sync
	wbytes, err := io.Copy(f, rd)
	if err != nil {
		return errors.WithStack(err)
	}
	// sanity check
	if wbytes != rd.Length() {
		return errors.Errorf("wrote %d bytes instead of the expected %d bytes", wbytes, rd.Length())
	}

	// Ignore error if filesystem does not support fsync.
	err = f.Sync()
	syncNotSup := err != nil && (errors.Is(err, syscall.ENOTSUP) || isMacENOTTY(err))
	if err != nil && !syncNotSup {
		return errors.WithStack(err)
	}

	// Close, then rename. Windows doesn't like the reverse order.
	if err = f.Close(); err != nil {
		return errors.WithStack(err)
	}
	if err = os.Rename(f.Name(), finalname); err != nil {
		return errors.WithStack(err)
	}

	// Now sync the directory to commit the Rename.
	if !syncNotSup {
		err = fsyncDir(dir)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// try to mark file as read-only to avoid accidental modifications
	// ignore if the operation fails as some filesystems don't allow the chmod call
	// e.g. exfat and network file systems with certain mount options
	err = setFileReadonly(finalname, b.Modes.File)
	if err != nil && !os.IsPermission(err) {
		return errors.WithStack(err)
	}

	return nil
}

var tempFile = os.CreateTemp // Overridden by test.

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (b *Local) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	return util.DefaultLoad(ctx, h, length, offset, b.openReader, fn)
}

func (b *Local) openReader(_ context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
	f, err := fs.Open(b.Filename(h))
	if err != nil {
		return nil, err
	}

	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	size := fi.Size()
	if size < offset+int64(length) {
		_ = f.Close()
		return nil, errTooShort
	}

	if offset > 0 {
		_, err = f.Seek(offset, 0)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
	}

	if length > 0 {
		return util.LimitReadCloser(f, int64(length)), nil
	}

	return f, nil
}

// Stat returns information about a blob.
func (b *Local) Stat(_ context.Context, h backend.Handle) (backend.FileInfo, error) {
	fi, err := fs.Stat(b.Filename(h))
	if err != nil {
		return backend.FileInfo{}, errors.WithStack(err)
	}

	return backend.FileInfo{Size: fi.Size(), Name: h.Name}, nil
}

// Remove removes the blob with the given name and type.
func (b *Local) Remove(_ context.Context, h backend.Handle) error {
	fn := b.Filename(h)

	// reset read-only flag
	err := fs.Chmod(fn, 0666)
	if err != nil && !os.IsPermission(err) {
		return errors.WithStack(err)
	}

	return fs.Remove(fn)
}

// List runs fn for each file in the backend which has the type t. When an
// error occurs (or fn returns an error), List stops and returns it.
func (b *Local) List(ctx context.Context, t backend.FileType, fn func(backend.FileInfo) error) (err error) {
	basedir, subdirs := b.Basedir(t)
	if subdirs {
		err = visitDirs(ctx, basedir, fn)
	} else {
		err = visitFiles(ctx, basedir, fn, false)
	}

	if b.IsNotExist(err) {
		debug.Log("ignoring non-existing directory")
		return nil
	}

	return err
}

// The following two functions are like filepath.Walk, but visit only one or
// two levels of directory structure (including dir itself as the first level).
// Also, visitDirs assumes it sees a directory full of directories, while
// visitFiles wants a directory full or regular files.
func visitDirs(ctx context.Context, dir string, fn func(backend.FileInfo) error) error {
	d, err := fs.Open(dir)
	if err != nil {
		return err
	}

	sub, err := d.Readdirnames(-1)
	if err != nil {
		// ignore subsequent errors
		_ = d.Close()
		return err
	}

	err = d.Close()
	if err != nil {
		return err
	}

	for _, f := range sub {
		err = visitFiles(ctx, filepath.Join(dir, f), fn, true)
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}

func visitFiles(ctx context.Context, dir string, fn func(backend.FileInfo) error, ignoreNotADirectory bool) error {
	d, err := fs.Open(dir)
	if err != nil {
		return err
	}

	if ignoreNotADirectory {
		fi, err := d.Stat()
		if err != nil || !fi.IsDir() {
			// ignore subsequent errors
			_ = d.Close()
			return err
		}
	}

	sub, err := d.Readdir(-1)
	if err != nil {
		// ignore subsequent errors
		_ = d.Close()
		return err
	}

	err = d.Close()
	if err != nil {
		return err
	}

	for _, fi := range sub {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := fn(backend.FileInfo{
			Name: fi.Name(),
			Size: fi.Size(),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Delete removes the repository and all files.
func (b *Local) Delete(_ context.Context) error {
	return fs.RemoveAll(b.Path)
}

// Close closes all open files.
func (b *Local) Close() error {
	// this does not need to do anything, all open files are closed within the
	// same function.
	return nil
}
