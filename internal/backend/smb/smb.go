package smb

import (
	"context"
	"hash"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hirochachacha/go-smb2"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/layout"
	"github.com/restic/restic/internal/backend/sema"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/valyala/fastrand"
)

// Backend stores data on an SMB endpoint.
type Backend struct {
	sem sema.Semaphore
	Config
	layout.Layout
	backend.Modes

	sessions int32
	poolMu   sync.Mutex
	pool     []*conn
	drain    *time.Timer // used to drain the pool when we stop using the connections
}

// make sure that *Backend implements backend.Backend
var _ restic.Backend = &Backend{}

const (
	defaultLayout = "default"
)

func open(ctx context.Context, cfg Config) (*Backend, error) {

	l, err := layout.ParseLayout(ctx, &layout.LocalFilesystem{}, cfg.Layout, defaultLayout, cfg.Path)
	if err != nil {
		return nil, err
	}

	sem, err := sema.New(cfg.Connections)
	if err != nil {
		return nil, err
	}

	b := &Backend{
		Config: cfg,
		sem:    sem,
		Layout: l,
	}

	debug.Log("open, config %#v", cfg)

	// set the pool drainer timer going
	if b.Config.IdleTimeout > 0 {
		b.drain = time.AfterFunc(b.Config.IdleTimeout, func() { _ = b.drainPool() })
	}

	cn, err := b.getConnection(ctx, b.ShareName)
	if err != nil {
		return nil, err
	}
	defer b.putConnection(&cn)

	stat, err := cn.smbShare.Stat(l.Filename(restic.Handle{Type: restic.ConfigFile}))
	m := backend.DeriveModesFromFileInfo(stat, err)
	debug.Log("using (%03O file, %03O dir) permissions", m.File, m.Dir)

	b.Modes = m

	return b, nil
}

// Open opens the local backend as specified by config.
func Open(ctx context.Context, cfg Config) (*Backend, error) {
	debug.Log("open local backend at %v (layout %q)", cfg.Path, cfg.Layout)
	return open(ctx, cfg)
}

// Create creates all the necessary files and directories for a new local
// backend at dir. Afterwards a new config blob should be created.
func Create(ctx context.Context, cfg Config) (*Backend, error) {
	debug.Log("create local backend at %v (layout %q)", cfg.Path, cfg.Layout)

	b, err := open(ctx, cfg)
	if err != nil {
		return nil, err
	}

	cn, err := b.getConnection(ctx, cfg.ShareName)
	if err != nil {
		return b, err
	}
	defer b.putConnection(&cn)

	// test if config file already exists
	_, err = cn.smbShare.Lstat(b.Filename(restic.Handle{Type: restic.ConfigFile}))
	if err == nil {
		return nil, errors.New("config file already exists")
	}

	// create paths for data and refs
	for _, d := range b.Paths() {
		err := cn.smbShare.MkdirAll(d, b.Modes.Dir)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return b, nil
}

func (b *Backend) Connections() uint {
	return b.Config.Connections
}

// Location returns this backend's location (the directory name).
func (b *Backend) Location() string {
	return b.Join(b.ShareName, b.Path)
}

// Hasher may return a hash function for calculating a content hash for the backend
func (b *Backend) Hasher() hash.Hash {
	return nil
}

// HasAtomicReplace returns whether Save() can atomically replace files
func (b *Backend) HasAtomicReplace() bool {
	return true
}

// IsNotExist returns true if the error is caused by a non existing file.
func (b *Backend) IsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}

// Join combines path components with slashes.
func (b *Backend) Join(p ...string) string {
	return path.Join(p...)
}

// Save stores data in the backend at the handle.
func (b *Backend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) (err error) {
	debug.Log("Save %v", h)
	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}

	finalname := b.Filename(h)
	dir := filepath.Dir(finalname)

	defer func() {
		// Mark non-retriable errors as such
		if errors.Is(err, syscall.ENOSPC) || os.IsPermission(err) {
			err = backoff.Permanent(err)
		}
	}()

	b.sem.GetToken()
	defer b.sem.ReleaseToken()

	// Create new file with a temporary name.
	tmpname := filepath.Base(finalname) + "-tmp-"

	b.addSession() // Show session in use
	defer b.removeSession()

	cn, err := b.getConnection(ctx, b.ShareName)
	if err != nil {
		return err
	}
	defer b.putConnection(&cn)

	f, err := b.CreateTemp(cn, dir, tmpname)

	if b.IsNotExist(err) {
		debug.Log("error %v: creating dir", err)

		// error is caused by a missing directory, try to create it
		mkdirErr := cn.smbShare.MkdirAll(dir, b.Modes.Dir)
		if mkdirErr != nil {
			debug.Log("error creating dir %v: %v", dir, mkdirErr)
		} else {
			// try again
			f, err = b.CreateTemp(cn, dir, tmpname)
		}
	}

	if err != nil {
		return errors.WithStack(err)
	}

	defer func(f *smb2.File) {
		if err != nil {
			_ = f.Close() // Double Close is harmless.
			// Remove after Rename is harmless: we embed the final name in the
			// temporary's name and no other goroutine will get the same data to
			// Save, so the temporary name should never be reused by another
			// goroutine.
			_ = cn.smbShare.Remove(f.Name())
		}
	}(f)

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
	// In this case the sync call is on the smb client's file.
	err = f.Sync()
	syncNotSup := err != nil && (errors.Is(err, syscall.ENOTSUP))
	if err != nil && !syncNotSup {
		return errors.WithStack(err)
	}

	// Close, then rename. Windows doesn't like the reverse order.
	if err = f.Close(); err != nil {
		return errors.WithStack(err)
	}
	if err = cn.smbShare.Rename(f.Name(), finalname); err != nil {
		return errors.WithStack(err)
	}

	// try to mark file as read-only to avoid accidential modifications
	// ignore if the operation fails as some filesystems don't allow the chmod call
	// e.g. exfat and network file systems with certain mount options
	err = cn.setFileReadonly(finalname, b.Modes.File)
	if err != nil && !os.IsPermission(err) {
		return errors.WithStack(err)
	}

	return nil
}

// set file to readonly
func (cn *conn) setFileReadonly(f string, mode os.FileMode) error {
	return cn.smbShare.Chmod(f, mode&^0222)
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (b *Backend) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	return backend.DefaultLoad(ctx, h, length, offset, b.openReader, fn)
}

func (b *Backend) openReader(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	debug.Log("Load %v, length %v, offset %v", h, length, offset)
	if err := h.Valid(); err != nil {
		return nil, backoff.Permanent(err)
	}

	if offset < 0 {
		return nil, errors.New("offset is negative")
	}

	b.addSession() // Show session in use
	defer b.removeSession()
	cn, err := b.getConnection(ctx, b.ShareName)
	if err != nil {
		return nil, err
	}
	defer b.putConnection(&cn)

	b.sem.GetToken()
	f, err := cn.smbShare.Open(b.Filename(h))
	if err != nil {
		b.sem.ReleaseToken()
		return nil, err
	}

	if offset > 0 {
		_, err = f.Seek(offset, 0)
		if err != nil {
			b.sem.ReleaseToken()
			_ = f.Close()
			return nil, err
		}
	}

	r := b.sem.ReleaseTokenOnClose(f, nil)

	if length > 0 {
		return backend.LimitReadCloser(r, int64(length)), nil
	}

	return r, nil
}

// Stat returns information about a blob.
func (b *Backend) Stat(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
	debug.Log("Stat %v", h)
	if err := h.Valid(); err != nil {
		return restic.FileInfo{}, backoff.Permanent(err)
	}

	b.sem.GetToken()
	defer b.sem.ReleaseToken()

	cn, err := b.getConnection(ctx, b.ShareName)
	if err != nil {
		return restic.FileInfo{}, err
	}
	defer b.putConnection(&cn)

	fi, err := cn.smbShare.Stat(b.Filename(h))
	if err != nil {
		return restic.FileInfo{}, errors.WithStack(err)
	}

	return restic.FileInfo{Size: fi.Size(), Name: h.Name}, nil
}

// Remove removes the blob with the given name and type.
func (b *Backend) Remove(ctx context.Context, h restic.Handle) error {
	debug.Log("Remove %v", h)
	fn := b.Filename(h)

	b.sem.GetToken()
	defer b.sem.ReleaseToken()

	cn, err := b.getConnection(ctx, b.ShareName)
	if err != nil {
		return err
	}
	defer b.putConnection(&cn)

	// reset read-only flag
	err = cn.smbShare.Chmod(fn, 0666)
	if err != nil && !os.IsPermission(err) {
		return errors.WithStack(err)
	}

	return cn.smbShare.Remove(fn)
}

// List runs fn for each file in the backend which has the type t. When an
// error occurs (or fn returns an error), List stops and returns it.
func (b *Backend) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) (err error) {
	debug.Log("List %v", t)

	cn, err := b.getConnection(ctx, b.ShareName)
	if err != nil {
		return err
	}
	defer b.putConnection(&cn)

	basedir, subdirs := b.Basedir(t)
	if subdirs {
		err = b.visitDirs(ctx, cn, basedir, fn)
	} else {
		err = b.visitFiles(ctx, cn, basedir, fn, false)
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
func (b *Backend) visitDirs(ctx context.Context, cn *conn, dir string, fn func(restic.FileInfo) error) error {
	d, err := cn.smbShare.Open(dir)
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
		err = b.visitFiles(ctx, cn, filepath.Join(dir, f), fn, true)
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (b *Backend) visitFiles(ctx context.Context, cn *conn, dir string, fn func(restic.FileInfo) error, ignoreNotADirectory bool) error {
	d, err := cn.smbShare.Open(dir)
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
func (b *Backend) Delete(ctx context.Context) error {
	debug.Log("Delete()")
	cn, err := b.getConnection(ctx, b.ShareName)
	if err != nil {
		return err
	}
	defer b.putConnection(&cn)
	return cn.smbShare.RemoveAll(b.Location())
}

// Close closes all open files.
func (b *Backend) Close() error {
	debug.Log("Close()")
	err := b.drainPool()
	return err
}

const (
	pathSeparator = '/' // Always using '/' for SMB
)

// CreateTemp creates a new temporary file in the directory dir,
// opens the file for reading and writing, and returns the resulting file.
// The filename is generated by taking pattern and adding a random string to the end.
// If pattern includes a "*", the random string replaces the last "*".
// If dir is the empty string, CreateTemp uses the default directory for temporary files, as returned by TempDir.
// Multiple programs or goroutines calling CreateTemp simultaneously will not choose the same file.
// The caller can use the file's Name method to find the pathname of the file.
// It is the caller's responsibility to remove the file when it is no longer needed.
func (b *Backend) CreateTemp(cn *conn, dir, pattern string) (*smb2.File, error) {
	if dir == "" {
		dir = os.TempDir()
	}

	prefix, suffix, err := prefixAndSuffix(pattern)
	if err != nil {
		return nil, &fs.PathError{Op: "createtemp", Path: pattern, Err: err}
	}
	prefix = joinPath(dir, prefix)

	try := 0
	for {
		name := prefix + nextRandom() + suffix
		f, err := cn.smbShare.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)

		if os.IsExist(err) {
			if try++; try < 10000 {
				continue
			}
			return nil, &fs.PathError{Op: "createtemp", Path: prefix + "*" + suffix, Err: fs.ErrExist}
		}
		return f, err
	}
}

var errPatternHasSeparator = errors.New("pattern contains path separator")

// prefixAndSuffix splits pattern by the last wildcard "*", if applicable,
// returning prefix as the part before "*" and suffix as the part after "*".
func prefixAndSuffix(pattern string) (prefix, suffix string, err error) {
	for i := 0; i < len(pattern); i++ {
		if IsPathSeparator(pattern[i]) {
			return "", "", errPatternHasSeparator
		}
	}
	if pos := lastIndex(pattern, '*'); pos != -1 {
		prefix, suffix = pattern[:pos], pattern[pos+1:]
	} else {
		prefix = pattern
	}
	return prefix, suffix, nil
}

// LastIndexByte from the strings package.
func lastIndex(s string, sep byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == sep {
			return i
		}
	}
	return -1
}

func nextRandom() string {
	return strconv.FormatUint(uint64(fastrand.Uint32()), 10)
}

func joinPath(dir, name string) string {
	if len(dir) > 0 && IsPathSeparator(dir[len(dir)-1]) {
		return dir + name
	}
	return dir + string(pathSeparator) + name
}

// IsPathSeparator reports whether c is a directory separator character.
func IsPathSeparator(c uint8) bool {
	// NOTE: Windows accepts / as path separator.
	return c == '\\' || c == '/'
}
