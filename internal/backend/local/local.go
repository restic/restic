package local

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"

	"github.com/cenkalti/backoff/v4"
)

// Local is a backend in a local directory.
type Local struct {
	Config
	backend.Layout
}

// ensure statically that *Local implements restic.Backend.
var _ restic.Backend = &Local{}

const defaultLayout = "default"

// Open opens the local backend as specified by config.
func Open(ctx context.Context, cfg Config) (*Local, error) {
	debug.Log("open local backend at %v (layout %q)", cfg.Path, cfg.Layout)
	l, err := backend.ParseLayout(ctx, &backend.LocalFilesystem{}, cfg.Layout, defaultLayout, cfg.Path)
	if err != nil {
		return nil, err
	}

	return &Local{Config: cfg, Layout: l}, nil
}

// Create creates all the necessary files and directories for a new local
// backend at dir. Afterwards a new config blob should be created.
func Create(ctx context.Context, cfg Config) (*Local, error) {
	debug.Log("create local backend at %v (layout %q)", cfg.Path, cfg.Layout)

	l, err := backend.ParseLayout(ctx, &backend.LocalFilesystem{}, cfg.Layout, defaultLayout, cfg.Path)
	if err != nil {
		return nil, err
	}

	be := &Local{
		Config: cfg,
		Layout: l,
	}

	// test if config file already exists
	_, err = fs.Lstat(be.Filename(restic.Handle{Type: restic.ConfigFile}))
	if err == nil {
		return nil, errors.New("config file already exists")
	}

	// create paths for data and refs
	for _, d := range be.Paths() {
		err := fs.MkdirAll(d, backend.Modes.Dir)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return be, nil
}

// Location returns this backend's location (the directory name).
func (b *Local) Location() string {
	return b.Path
}

// IsNotExist returns true if the error is caused by a non existing file.
func (b *Local) IsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}

// Save stores data in the backend at the handle.
func (b *Local) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) (err error) {
	debug.Log("Save %v", h)
	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}

	filename := b.Filename(h)

	defer func() {
		// Mark non-retriable errors as such
		if errors.Is(err, syscall.ENOSPC) || os.IsPermission(err) {
			err = backoff.Permanent(err)
		}
	}()

	// create new file
	f, err := openFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, backend.Modes.File)

	if b.IsNotExist(err) {
		debug.Log("error %v: creating dir", err)

		// error is caused by a missing directory, try to create it
		mkdirErr := os.MkdirAll(filepath.Dir(filename), backend.Modes.Dir)
		if mkdirErr != nil {
			debug.Log("error creating dir %v: %v", filepath.Dir(filename), mkdirErr)
		} else {
			// try again
			f, err = openFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, backend.Modes.File)
		}
	}

	if err != nil {
		return errors.WithStack(err)
	}

	// save data, then sync
	wbytes, err := io.Copy(f, rd)
	if err != nil {
		_ = f.Close()
		return errors.WithStack(err)
	}
	// sanity check
	if wbytes != rd.Length() {
		_ = f.Close()
		return errors.Errorf("wrote %d bytes instead of the expected %d bytes", wbytes, rd.Length())
	}

	if err = f.Sync(); err != nil {
		pathErr, ok := err.(*os.PathError)
		isNotSupported := ok && pathErr.Op == "sync" && pathErr.Err == syscall.ENOTSUP
		// ignore error if filesystem does not support the sync operation
		if !isNotSupported {
			_ = f.Close()
			return errors.WithStack(err)
		}
	}

	err = f.Close()
	if err != nil {
		return errors.WithStack(err)
	}

	// try to mark file as read-only to avoid accidential modifications
	// ignore if the operation fails as some filesystems don't allow the chmod call
	// e.g. exfat and network file systems with certain mount options
	err = setFileReadonly(filename, backend.Modes.File)
	if err != nil && !os.IsPermission(err) {
		return errors.WithStack(err)
	}

	return nil
}

var openFile = fs.OpenFile // Overridden by test.

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (b *Local) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	return backend.DefaultLoad(ctx, h, length, offset, b.openReader, fn)
}

func (b *Local) openReader(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	debug.Log("Load %v, length %v, offset %v", h, length, offset)
	if err := h.Valid(); err != nil {
		return nil, backoff.Permanent(err)
	}

	if offset < 0 {
		return nil, errors.New("offset is negative")
	}

	f, err := fs.Open(b.Filename(h))
	if err != nil {
		return nil, err
	}

	if offset > 0 {
		_, err = f.Seek(offset, 0)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
	}

	if length > 0 {
		return backend.LimitReadCloser(f, int64(length)), nil
	}

	return f, nil
}

// Stat returns information about a blob.
func (b *Local) Stat(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
	debug.Log("Stat %v", h)
	if err := h.Valid(); err != nil {
		return restic.FileInfo{}, backoff.Permanent(err)
	}

	fi, err := fs.Stat(b.Filename(h))
	if err != nil {
		return restic.FileInfo{}, errors.WithStack(err)
	}

	return restic.FileInfo{Size: fi.Size(), Name: h.Name}, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (b *Local) Test(ctx context.Context, h restic.Handle) (bool, error) {
	debug.Log("Test %v", h)
	_, err := fs.Stat(b.Filename(h))
	if err != nil {
		if b.IsNotExist(err) {
			return false, nil
		}
		return false, errors.WithStack(err)
	}

	return true, nil
}

// Remove removes the blob with the given name and type.
func (b *Local) Remove(ctx context.Context, h restic.Handle) error {
	debug.Log("Remove %v", h)
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
func (b *Local) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) (err error) {
	debug.Log("List %v", t)

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
func visitDirs(ctx context.Context, dir string, fn func(restic.FileInfo) error) error {
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

func visitFiles(ctx context.Context, dir string, fn func(restic.FileInfo) error, ignoreNotADirectory bool) error {
	d, err := fs.Open(dir)
	if err != nil {
		return err
	}

	if ignoreNotADirectory {
		fi, err := d.Stat()
		if err != nil || !fi.IsDir() {
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

		err := fn(restic.FileInfo{
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
func (b *Local) Delete(ctx context.Context) error {
	debug.Log("Delete()")
	return fs.RemoveAll(b.Path)
}

// Close closes all open files.
func (b *Local) Close() error {
	debug.Log("Close()")
	// this does not need to do anything, all open files are closed within the
	// same function.
	return nil
}
