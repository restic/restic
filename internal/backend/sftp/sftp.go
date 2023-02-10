package sftp

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/layout"
	"github.com/restic/restic/internal/backend/sema"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/cenkalti/backoff/v4"
	"github.com/pkg/sftp"
	"golang.org/x/sync/errgroup"
)

// SFTP is a backend in a directory accessed via SFTP.
type SFTP struct {
	c *sftp.Client
	p string

	cmd    *exec.Cmd
	result <-chan error

	posixRename bool

	sem sema.Semaphore
	layout.Layout
	Config
	backend.Modes
}

var _ restic.Backend = &SFTP{}

const defaultLayout = "default"

func startClient(cfg Config) (*SFTP, error) {
	program, args, err := buildSSHCommand(cfg)
	if err != nil {
		return nil, err
	}

	debug.Log("start client %v %v", program, args)
	// Connect to a remote host and request the sftp subsystem via the 'ssh'
	// command.  This assumes that passwordless login is correctly configured.
	cmd := exec.Command(program, args...)

	// prefix the errors with the program name
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, errors.Wrap(err, "cmd.StderrPipe")
	}

	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			fmt.Fprintf(os.Stderr, "subprocess %v: %v\n", program, sc.Text())
		}
	}()

	// get stdin and stdout
	wr, err := cmd.StdinPipe()
	if err != nil {
		return nil, errors.Wrap(err, "cmd.StdinPipe")
	}
	rd, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, "cmd.StdoutPipe")
	}

	bg, err := backend.StartForeground(cmd)
	if err != nil {
		if backend.IsErrDot(err) {
			return nil, errors.Errorf("cannot implicitly run relative executable %v found in current directory, use -o sftp.command=./<command> to override", cmd.Path)
		}
		return nil, err
	}

	// wait in a different goroutine
	ch := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		debug.Log("ssh command exited, err %v", err)
		for {
			ch <- errors.Wrap(err, "ssh command exited")
		}
	}()

	// open the SFTP session
	client, err := sftp.NewClientPipe(rd, wr)
	if err != nil {
		return nil, errors.Errorf("unable to start the sftp session, error: %v", err)
	}

	err = bg()
	if err != nil {
		return nil, errors.Wrap(err, "bg")
	}

	_, posixRename := client.HasExtension("posix-rename@openssh.com")
	return &SFTP{c: client, cmd: cmd, result: ch, posixRename: posixRename}, nil
}

// clientError returns an error if the client has exited. Otherwise, nil is
// returned immediately.
func (r *SFTP) clientError() error {
	select {
	case err := <-r.result:
		debug.Log("client has exited with err %v", err)
		return backoff.Permanent(err)
	default:
	}

	return nil
}

// Open opens an sftp backend as described by the config by running
// "ssh" with the appropriate arguments (or cfg.Command, if set).
func Open(ctx context.Context, cfg Config) (*SFTP, error) {
	debug.Log("open backend with config %#v", cfg)

	sftp, err := startClient(cfg)
	if err != nil {
		debug.Log("unable to start program: %v", err)
		return nil, err
	}

	return open(ctx, sftp, cfg)
}

func open(ctx context.Context, sftp *SFTP, cfg Config) (*SFTP, error) {
	sem, err := sema.New(cfg.Connections)
	if err != nil {
		return nil, err
	}

	sftp.Layout, err = layout.ParseLayout(ctx, sftp, cfg.Layout, defaultLayout, cfg.Path)
	if err != nil {
		return nil, err
	}

	debug.Log("layout: %v\n", sftp.Layout)

	fi, err := sftp.c.Stat(sftp.Layout.Filename(restic.Handle{Type: restic.ConfigFile}))
	m := backend.DeriveModesFromFileInfo(fi, err)
	debug.Log("using (%03O file, %03O dir) permissions", m.File, m.Dir)

	sftp.Config = cfg
	sftp.p = cfg.Path
	sftp.sem = sem
	sftp.Modes = m
	return sftp, nil
}

func (r *SFTP) mkdirAllDataSubdirs(ctx context.Context, nconn uint) error {
	// Run multiple MkdirAll calls concurrently. These involve multiple
	// round-trips and we do a lot of them, so this whole operation can be slow
	// on high-latency links.
	g, _ := errgroup.WithContext(ctx)
	// Use errgroup's built-in semaphore, because r.sem is not initialized yet.
	g.SetLimit(int(nconn))

	for _, d := range r.Paths() {
		d := d
		g.Go(func() error {
			// First try Mkdir. For most directories in Paths, this takes one
			// round trip, not counting duplicate parent creations causes by
			// concurrency. MkdirAll first does Stat, then recursive MkdirAll
			// on the parent, so calls typically take three round trips.
			if err := r.c.Mkdir(d); err == nil {
				return nil
			}
			return r.c.MkdirAll(d)
		})
	}

	return g.Wait()
}

// Join combines path components with slashes (according to the sftp spec).
func (r *SFTP) Join(p ...string) string {
	return path.Join(p...)
}

// ReadDir returns the entries for a directory.
func (r *SFTP) ReadDir(ctx context.Context, dir string) ([]os.FileInfo, error) {
	fi, err := r.c.ReadDir(dir)

	// sftp client does not specify dir name on error, so add it here
	err = errors.Wrapf(err, "(%v)", dir)

	return fi, err
}

// IsNotExist returns true if the error is caused by a not existing file.
func (r *SFTP) IsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}

func buildSSHCommand(cfg Config) (cmd string, args []string, err error) {
	if cfg.Command != "" {
		args, err := backend.SplitShellStrings(cfg.Command)
		if err != nil {
			return "", nil, err
		}

		return args[0], args[1:], nil
	}

	cmd = "ssh"

	host, port := cfg.Host, cfg.Port

	args = []string{host}
	if port != "" {
		args = append(args, "-p", port)
	}
	if cfg.User != "" {
		args = append(args, "-l")
		args = append(args, cfg.User)
	}
	args = append(args, "-s")
	args = append(args, "sftp")
	return cmd, args, nil
}

// Create creates an sftp backend as described by the config by running "ssh"
// with the appropriate arguments (or cfg.Command, if set).
func Create(ctx context.Context, cfg Config) (*SFTP, error) {
	sftp, err := startClient(cfg)
	if err != nil {
		debug.Log("unable to start program: %v", err)
		return nil, err
	}

	sftp.Layout, err = layout.ParseLayout(ctx, sftp, cfg.Layout, defaultLayout, cfg.Path)
	if err != nil {
		return nil, err
	}

	sftp.Modes = backend.DefaultModes

	// test if config file already exists
	_, err = sftp.c.Lstat(sftp.Layout.Filename(restic.Handle{Type: restic.ConfigFile}))
	if err == nil {
		return nil, errors.New("config file already exists")
	}

	// create paths for data and refs
	if err = sftp.mkdirAllDataSubdirs(ctx, cfg.Connections); err != nil {
		return nil, err
	}

	// repurpose existing connection
	return open(ctx, sftp, cfg)
}

func (r *SFTP) Connections() uint {
	return r.Config.Connections
}

// Location returns this backend's location (the directory name).
func (r *SFTP) Location() string {
	return r.p
}

// Hasher may return a hash function for calculating a content hash for the backend
func (r *SFTP) Hasher() hash.Hash {
	return nil
}

// HasAtomicReplace returns whether Save() can atomically replace files
func (r *SFTP) HasAtomicReplace() bool {
	return r.posixRename
}

// Join joins the given paths and cleans them afterwards. This always uses
// forward slashes, which is required by sftp.
func Join(parts ...string) string {
	return path.Clean(path.Join(parts...))
}

// tempSuffix generates a random string suffix that should be sufficiently long
// to avoid accidential conflicts
func tempSuffix() string {
	var nonce [16]byte
	_, err := rand.Read(nonce[:])
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(nonce[:])
}

// Save stores data in the backend at the handle.
func (r *SFTP) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	debug.Log("Save %v", h)
	if err := r.clientError(); err != nil {
		return err
	}

	if err := h.Valid(); err != nil {
		return backoff.Permanent(err)
	}

	filename := r.Filename(h)
	tmpFilename := filename + "-restic-temp-" + tempSuffix()
	dirname := r.Dirname(h)

	r.sem.GetToken()
	defer r.sem.ReleaseToken()

	// create new file
	f, err := r.c.OpenFile(tmpFilename, os.O_CREATE|os.O_EXCL|os.O_WRONLY)

	if r.IsNotExist(err) {
		// error is caused by a missing directory, try to create it
		mkdirErr := r.c.MkdirAll(r.Dirname(h))
		if mkdirErr != nil {
			debug.Log("error creating dir %v: %v", r.Dirname(h), mkdirErr)
		} else {
			// try again
			f, err = r.c.OpenFile(tmpFilename, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
		}
	}

	// pkg/sftp doesn't allow creating with a mode.
	// Chmod while the file is still empty.
	if err == nil {
		err = f.Chmod(r.Modes.File)
	}
	if err != nil {
		return errors.Wrap(err, "OpenFile")
	}

	defer func() {
		if err == nil {
			return
		}

		// Try not to leave a partial file behind.
		rmErr := r.c.Remove(f.Name())
		if rmErr != nil {
			debug.Log("sftp: failed to remove broken file %v: %v",
				f.Name(), rmErr)
		}
	}()

	// save data, make sure to use the optimized sftp upload method
	wbytes, err := f.ReadFrom(rd)
	if err != nil {
		_ = f.Close()
		err = r.checkNoSpace(dirname, rd.Length(), err)
		return errors.Wrap(err, "Write")
	}

	// sanity check
	if wbytes != rd.Length() {
		_ = f.Close()
		return errors.Errorf("wrote %d bytes instead of the expected %d bytes", wbytes, rd.Length())
	}

	err = f.Close()
	if err != nil {
		return errors.Wrap(err, "Close")
	}

	// Prefer POSIX atomic rename if available.
	if r.posixRename {
		err = r.c.PosixRename(tmpFilename, filename)
	} else {
		err = r.c.Rename(tmpFilename, filename)
	}
	return errors.Wrap(err, "Rename")
}

// checkNoSpace checks if err was likely caused by lack of available space
// on the remote, and if so, makes it permanent.
func (r *SFTP) checkNoSpace(dir string, size int64, origErr error) error {
	// The SFTP protocol has a message for ENOSPC,
	// but pkg/sftp doesn't export it and OpenSSH's sftp-server
	// sends FX_FAILURE instead.

	e, ok := origErr.(*sftp.StatusError)
	_, hasExt := r.c.HasExtension("statvfs@openssh.com")
	if !ok || e.FxCode() != sftp.ErrSSHFxFailure || !hasExt {
		return origErr
	}

	fsinfo, err := r.c.StatVFS(dir)
	if err != nil {
		debug.Log("sftp: StatVFS returned %v", err)
		return origErr
	}
	if fsinfo.Favail == 0 || fsinfo.Frsize*fsinfo.Bavail < uint64(size) {
		err := errors.New("sftp: no space left on device")
		return backoff.Permanent(err)
	}
	return origErr
}

// Load runs fn with a reader that yields the contents of the file at h at the
// given offset.
func (r *SFTP) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	return backend.DefaultLoad(ctx, h, length, offset, r.openReader, fn)
}

// wrapReader wraps an io.ReadCloser to run an additional function on Close.
type wrapReader struct {
	io.ReadCloser
	io.WriterTo
	f func()
}

func (wr *wrapReader) Close() error {
	err := wr.ReadCloser.Close()
	wr.f()
	return err
}

func (r *SFTP) openReader(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	debug.Log("Load %v, length %v, offset %v", h, length, offset)
	if err := h.Valid(); err != nil {
		return nil, backoff.Permanent(err)
	}

	if offset < 0 {
		return nil, errors.New("offset is negative")
	}

	r.sem.GetToken()
	f, err := r.c.Open(r.Filename(h))
	if err != nil {
		r.sem.ReleaseToken()
		return nil, err
	}

	if offset > 0 {
		_, err = f.Seek(offset, 0)
		if err != nil {
			r.sem.ReleaseToken()
			_ = f.Close()
			return nil, err
		}
	}

	// use custom close wrapper to also provide WriteTo() on the wrapper
	rd := &wrapReader{
		ReadCloser: f,
		WriterTo:   f,
		f: func() {
			r.sem.ReleaseToken()
		},
	}

	if length > 0 {
		// unlimited reads usually use io.Copy which needs WriteTo support at the underlying reader
		// limited reads are usually combined with io.ReadFull which reads all required bytes into a buffer in one go
		return backend.LimitReadCloser(rd, int64(length)), nil
	}

	return rd, nil
}

// Stat returns information about a blob.
func (r *SFTP) Stat(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
	debug.Log("Stat(%v)", h)
	if err := r.clientError(); err != nil {
		return restic.FileInfo{}, err
	}

	if err := h.Valid(); err != nil {
		return restic.FileInfo{}, backoff.Permanent(err)
	}

	r.sem.GetToken()
	defer r.sem.ReleaseToken()

	fi, err := r.c.Lstat(r.Filename(h))
	if err != nil {
		return restic.FileInfo{}, errors.Wrap(err, "Lstat")
	}

	return restic.FileInfo{Size: fi.Size(), Name: h.Name}, nil
}

// Remove removes the content stored at name.
func (r *SFTP) Remove(ctx context.Context, h restic.Handle) error {
	debug.Log("Remove(%v)", h)
	if err := r.clientError(); err != nil {
		return err
	}

	r.sem.GetToken()
	defer r.sem.ReleaseToken()

	return r.c.Remove(r.Filename(h))
}

// List runs fn for each file in the backend which has the type t. When an
// error occurs (or fn returns an error), List stops and returns it.
func (r *SFTP) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
	debug.Log("List %v", t)

	basedir, subdirs := r.Basedir(t)
	walker := r.c.Walk(basedir)
	for {
		r.sem.GetToken()
		ok := walker.Step()
		r.sem.ReleaseToken()
		if !ok {
			break
		}

		if walker.Err() != nil {
			if r.IsNotExist(walker.Err()) {
				debug.Log("ignoring non-existing directory")
				return nil
			}
			return walker.Err()
		}

		if walker.Path() == basedir {
			continue
		}

		if walker.Stat().IsDir() && !subdirs {
			walker.SkipDir()
			continue
		}

		fi := walker.Stat()
		if !fi.Mode().IsRegular() {
			continue
		}

		debug.Log("send %v\n", path.Base(walker.Path()))

		rfi := restic.FileInfo{
			Name: path.Base(walker.Path()),
			Size: fi.Size(),
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := fn(rfi)
		if err != nil {
			return err
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return ctx.Err()
}

var closeTimeout = 2 * time.Second

// Close closes the sftp connection and terminates the underlying command.
func (r *SFTP) Close() error {
	debug.Log("Close")
	if r == nil {
		return nil
	}

	err := r.c.Close()
	debug.Log("Close returned error %v", err)

	// wait for closeTimeout before killing the process
	select {
	case err := <-r.result:
		return err
	case <-time.After(closeTimeout):
	}

	if err := r.cmd.Process.Kill(); err != nil {
		return err
	}

	// get the error, but ignore it
	<-r.result
	return nil
}

func (r *SFTP) deleteRecursive(ctx context.Context, name string) error {
	entries, err := r.ReadDir(ctx, name)
	if err != nil {
		return errors.Wrap(err, "ReadDir")
	}

	for _, fi := range entries {
		itemName := r.Join(name, fi.Name())
		if fi.IsDir() {
			err := r.deleteRecursive(ctx, itemName)
			if err != nil {
				return errors.Wrap(err, "ReadDir")
			}

			err = r.c.RemoveDirectory(itemName)
			if err != nil {
				return errors.Wrap(err, "RemoveDirectory")
			}

			continue
		}

		err := r.c.Remove(itemName)
		if err != nil {
			return errors.Wrap(err, "ReadDir")
		}
	}

	return nil
}

// Delete removes all data in the backend.
func (r *SFTP) Delete(ctx context.Context) error {
	return r.deleteRecursive(ctx, r.p)
}
