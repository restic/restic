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
	"sync"
	"sync/atomic"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/layout"
	"github.com/restic/restic/internal/backend/limiter"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/backend/util"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/feature"
	"github.com/restic/restic/internal/terminal"

	"github.com/cenkalti/backoff/v4"
	"github.com/pkg/sftp"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

// SFTP is a backend in a directory accessed via SFTP.
type SFTP struct {
	p string

	// Connection state, guarded by connMu.
	connMu sync.Mutex
	cs *connState
	closed atomic.Bool // set by Close(); prevents reconnect after shutdown

	// Reconnect coordination.
	connGen  atomic.Uint64
	sfGroup  singleflight.Group
	budget   reconnectBudget
	errorLog func(string, ...interface{})

	layout.Layout
	Config
	util.Modes
}

var _ backend.Backend = &SFTP{}

var errTooShort = fmt.Errorf("file is too short")

func NewFactory() location.Factory {
	return location.NewLimitedBackendFactory("sftp", ParseConfig, location.NoPassword, limiter.WrapBackendConstructor(Create), limiter.WrapBackendConstructor(Open))
}

func startClient(cfg Config, errorLog func(string, ...interface{})) (*connState, error) {
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

	done := make(chan struct{})
	// On early error, kill the subprocess (if started) and signal
	// auxiliary goroutines to stop.
	success := false
	defer func() {
		if !success {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
			close(done)
		}
	}()

	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			select {
			case <-done:
				return
			default:
			}
			errorLog("subprocess %v: %v\n", program, sc.Text())
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

	bg, err := terminal.StartForeground(cmd)
	if err != nil {
		if errors.Is(err, exec.ErrDot) {
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
			select {
			case ch <- errors.Wrap(err, "ssh command exited"):
			case <-done:
				return
			}
		}
	}()

	// open the SFTP session
	client, err := sftp.NewClientPipe(rd, wr,
		// write multiple packets (32kb) in parallel per file
		// not strictly necessary as we use ReadFromWithConcurrency
		sftp.UseConcurrentWrites(true),
		// increase send buffer per file to 4MB
		sftp.MaxConcurrentRequestsPerFile(128))
	if err != nil {
		return nil, errors.Errorf("unable to start the sftp session, error: %v", err)
	}

	err = bg()
	if err != nil {
		return nil, errors.Wrap(err, "bg")
	}

	_, posixRename := client.HasExtension("posix-rename@openssh.com")
	success = true
	return &connState{
		client:      client,
		cmd:         cmd.Process,
		result:      ch,
		posixRename: posixRename,
		done:        done,
	}, nil
}

// Open opens an sftp backend as described by the config by running
// "ssh" with the appropriate arguments (or cfg.Command, if set).
func Open(_ context.Context, cfg Config, errorLog func(string, ...interface{})) (*SFTP, error) {
	debug.Log("open backend with config %#v", cfg)

	cs, err := startClient(cfg, errorLog)
	if err != nil {
		debug.Log("unable to start program: %v", err)
		return nil, err
	}

	be := &SFTP{
		cs:    cs,
		Layout:   layout.NewDefaultLayout(cfg.Path, path.Join),
		errorLog: errorLog,
		budget:   newReconnectBudget(),
	}

	return openBackend(be, cfg)
}

func openBackend(be *SFTP, cfg Config) (*SFTP, error) {
	cs := be.conn()
	fi, err := cs.client.Stat(be.Layout.Filename(backend.Handle{Type: backend.ConfigFile}))
	m := util.DeriveModesFromFileInfo(fi, err)
	debug.Log("using (%03O file, %03O dir) permissions", m.File, m.Dir)

	be.Config = cfg
	be.p = cfg.Path
	be.Modes = m
	return be, nil
}

func (r *SFTP) mkdirAllDataSubdirs(ctx context.Context, nconn uint) error {
	cs := r.conn()

	// Run multiple MkdirAll calls concurrently. These involve multiple
	// round-trips and we do a lot of them, so this whole operation can be slow
	// on high-latency links.
	g, _ := errgroup.WithContext(ctx)
	// Use errgroup's built-in semaphore, because r.sem is not initialized yet.
	g.SetLimit(int(nconn))

	for _, d := range r.Paths() {
		g.Go(func() error {
			// First try Mkdir. For most directories in Paths, this takes one
			// round trip, not counting duplicate parent creations causes by
			// concurrency. MkdirAll first does Stat, then recursive MkdirAll
			// on the parent, so calls typically take three round trips.
			if err := cs.client.Mkdir(d); err == nil {
				return nil
			}
			return errors.Wrapf(cs.client.MkdirAll(d), "MkdirAll %v", d)
		})
	}

	return g.Wait()
}

// IsNotExist returns true if the error is caused by a not existing file.
func (r *SFTP) IsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}

func (r *SFTP) IsPermanentError(err error) bool {
	return r.IsNotExist(err) || errors.Is(err, errTooShort) || errors.Is(err, os.ErrPermission)
}

func buildSSHCommand(cfg Config) (cmd string, args []string, err error) {
	if cfg.Command != "" {
		args, err := backend.SplitShellStrings(cfg.Command)
		if err != nil {
			return "", nil, err
		}
		if cfg.Args != "" {
			return "", nil, errors.New("cannot specify both sftp.command and sftp.args options")
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
		args = append(args, "-l", cfg.User)
	}

	if cfg.Args != "" {
		a, err := backend.SplitShellStrings(cfg.Args)
		if err != nil {
			return "", nil, err
		}

		args = append(args, a...)
	}

	args = append(args, "-s", "sftp")
	return cmd, args, nil
}

// Create creates an sftp backend as described by the config by running "ssh"
// with the appropriate arguments (or cfg.Command, if set).
func Create(ctx context.Context, cfg Config, errorLog func(string, ...interface{})) (*SFTP, error) {
	cs, err := startClient(cfg, errorLog)
	if err != nil {
		debug.Log("unable to start program: %v", err)
		return nil, err
	}

	be := &SFTP{
		cs:    cs,
		Layout:   layout.NewDefaultLayout(cfg.Path, path.Join),
		Modes:    util.DefaultModes,
		errorLog: errorLog,
		budget:   newReconnectBudget(),
	}

	// test if config file already exists
	_, err = cs.client.Lstat(be.Layout.Filename(backend.Handle{Type: backend.ConfigFile}))
	if err == nil {
		return nil, errors.New("config file already exists")
	}

	// create paths for data and refs
	if err = be.mkdirAllDataSubdirs(ctx, cfg.Connections); err != nil {
		return nil, err
	}

	// repurpose existing connection
	return openBackend(be, cfg)
}

func (r *SFTP) Properties() backend.Properties {
	cs := r.conn()
	posixRename := false
	if cs != nil {
		posixRename = cs.posixRename
	}
	return backend.Properties{
		Connections:      r.Config.Connections,
		HasAtomicReplace: posixRename,
	}
}

// Hasher may return a hash function for calculating a content hash for the backend
func (r *SFTP) Hasher() hash.Hash {
	return nil
}

// tempSuffix generates a random string suffix that should be sufficiently long
// to avoid accidental conflicts
func tempSuffix() string {
	var nonce [16]byte
	_, err := rand.Read(nonce[:])
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(nonce[:])
}

func setFileReadonly(client *sftp.Client, path string, mode os.FileMode) error {
	// clear owner/group/other write bits
	readonlyMode := mode &^ 0o222
	err := client.Chmod(path, readonlyMode)

	// if the operation is not supported in the sftp server we ignore it.
	if errors.Is(err, sftp.ErrSSHFxOpUnsupported) {
		return nil
	}
	return err
}

// Save stores data in the backend at the handle.
func (r *SFTP) Save(_ context.Context, h backend.Handle, rd backend.RewindReader) error {
	return r.saveWithRetry(h, rd)
}

// saveWithRetry performs a Save with one reconnect retry on transient disconnect.
func (r *SFTP) saveWithRetry(h backend.Handle, rd backend.RewindReader) error {
	cs, gen, err := r.getConn()
	if err != nil {
		return err
	}

	err = r.doSave(cs, h, rd)
	if err == nil || r.Config.Reconnect == 0 || !isTransientDisconnect(err) {
		return err
	}

	debug.Log("transient disconnect during Save, attempting reconnect (gen %d): %v", gen, err)

	newCS, reconnErr := r.ensureConnected(gen)
	if reconnErr != nil {
		return reconnErr
	}

	if err := rd.Rewind(); err != nil {
		return errors.Wrap(err, "Rewind")
	}

	return r.doSave(newCS, h, rd)
}

// doSave performs a single Save attempt with the given connection state.
func (r *SFTP) doSave(cs *connState, h backend.Handle, rd backend.RewindReader) error {
	filename := r.Filename(h)
	tmpFilename := filename + "-restic-temp-" + tempSuffix()
	dirname := r.Dirname(h)

	// create new file
	f, err := cs.client.OpenFile(tmpFilename, os.O_CREATE|os.O_EXCL|os.O_WRONLY)

	if r.IsNotExist(err) {
		// error is caused by a missing directory, try to create it
		mkdirErr := cs.client.MkdirAll(r.Dirname(h))
		if mkdirErr != nil {
			debug.Log("error creating dir %v: %v", r.Dirname(h), mkdirErr)
		} else {
			// try again
			f, err = cs.client.OpenFile(tmpFilename, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
		}
	}

	if err != nil {
		return errors.Wrapf(err, "OpenFile %v", tmpFilename)
	}

	defer func() {
		if err == nil {
			return
		}

		// Try not to leave a partial file behind.
		rmErr := cs.client.Remove(f.Name())
		if rmErr != nil {
			debug.Log("sftp: failed to remove broken file %v: %v",
				f.Name(), rmErr)
		}
	}()

	// pkg/sftp doesn't allow creating with a mode.
	// Chmod while the file is still empty.
	err = f.Chmod(r.Modes.File)
	if err != nil {
		return errors.Wrapf(err, "Chmod %v", tmpFilename)
	}

	// save data, make sure to use the optimized sftp upload method
	wbytes, err := f.ReadFromWithConcurrency(rd, 0)
	if err != nil {
		_ = f.Close()
		// Skip checkNoSpace on transient disconnects — the connection is
		// dead and StatVFS would also fail.
		if !isTransientDisconnect(err) {
			err = r.checkNoSpace(cs, dirname, rd.Length(), err)
		}
		return errors.Wrapf(err, "Write %v", tmpFilename)
	}

	// sanity check
	if wbytes != rd.Length() {
		_ = f.Close()
		return errors.Errorf("Write %v: wrote %d bytes instead of the expected %d bytes", tmpFilename, wbytes, rd.Length())
	}
	err = f.Close()
	if err != nil {
		return errors.Wrapf(err, "Close %v", tmpFilename)
	}

	// Prefer POSIX atomic rename if available.
	if cs.posixRename {
		err = cs.client.PosixRename(tmpFilename, filename)
	} else {
		err = cs.client.Rename(tmpFilename, filename)
	}
	if err != nil {
		return errors.Wrapf(err, "Rename %v", tmpFilename)
	}

	err = setFileReadonly(cs.client, filename, r.Modes.File)
	if err != nil {
		return errors.Errorf("sftp setFileReadonly: %v", err)
	}

	return nil
}

// checkNoSpace checks if err was likely caused by lack of available space
// on the remote, and if so, makes it permanent.
func (r *SFTP) checkNoSpace(cs *connState, dir string, size int64, origErr error) error {
	// The SFTP protocol has a message for ENOSPC,
	// but pkg/sftp doesn't export it and OpenSSH's sftp-server
	// sends FX_FAILURE instead.

	e, ok := origErr.(*sftp.StatusError)
	_, hasExt := cs.client.HasExtension("statvfs@openssh.com")
	if !ok || e.FxCode() != sftp.ErrSSHFxFailure || !hasExt {
		return origErr
	}

	fsinfo, err := cs.client.StatVFS(dir)
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
func (r *SFTP) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	return r.withRetry(func(cs *connState) error {
		return util.DefaultLoad(ctx, h, length, offset, r.openReaderWith(cs), func(rd io.Reader) error {
			if length == 0 || !feature.Flag.Enabled(feature.BackendErrorRedesign) {
				return fn(rd)
			}

			// there is no direct way to efficiently check whether the file is too short
			// rd is already a LimitedReader which can be used to track the number of bytes read
			err := fn(rd)

			// check the underlying reader to be agnostic to however fn() handles the returned error
			_, rderr := rd.Read([]byte{0})
			if rderr == io.EOF && rd.(*util.LimitedReadCloser).N != 0 {
				// file is too short
				return fmt.Errorf("%w: %v", errTooShort, err)
			}

			return err
		})
	})
}

// openReaderWith returns an openReader function bound to the given connState.
func (r *SFTP) openReaderWith(cs *connState) func(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
	return func(_ context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
		return r.doOpenReader(cs, h, length, offset)
	}
}

func (r *SFTP) doOpenReader(cs *connState, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
	f, err := cs.client.Open(r.Filename(h))
	if err != nil {
		return nil, errors.Wrapf(err, "Open %v", r.Filename(h))
	}

	if offset > 0 {
		_, err = f.Seek(offset, 0)
		if err != nil {
			_ = f.Close()
			return nil, errors.Wrapf(err, "Seek %v", r.Filename(h))
		}
	}

	if length > 0 {
		// unlimited reads usually use io.Copy which needs WriteTo support at the underlying reader
		// limited reads are usually combined with io.ReadFull which reads all required bytes into a buffer in one go
		return util.LimitReadCloser(f, int64(length)), nil
	}

	return f, nil
}

// Stat returns information about a blob.
func (r *SFTP) Stat(_ context.Context, h backend.Handle) (backend.FileInfo, error) {
	var fi backend.FileInfo
	err := r.withRetry(func(cs *connState) error {
		osfi, err := cs.client.Lstat(r.Filename(h))
		if err != nil {
			return errors.Wrapf(err, "Lstat %v", r.Filename(h))
		}
		fi = backend.FileInfo{Size: osfi.Size(), Name: h.Name}
		return nil
	})
	return fi, err
}

// Remove removes the content stored at name.
func (r *SFTP) Remove(_ context.Context, h backend.Handle) error {
	return r.withRetry(func(cs *connState) error {
		return errors.Wrapf(cs.client.Remove(r.Filename(h)), "Remove %v", r.Filename(h))
	})
}

// List runs fn for each file in the backend which has the type t. When an
// error occurs (or fn returns an error), List stops and returns it.
//
// List does not retry internally (restarting would deliver duplicates to fn).
// On transient disconnect it triggers a reconnect so that the outer retry
// backend's next attempt gets a fresh connection.
func (r *SFTP) List(ctx context.Context, t backend.FileType, fn func(backend.FileInfo) error) error {
	cs, gen, err := r.getConn()
	if err != nil {
		return err
	}

	basedir, subdirs := r.Basedir(t)
	walker := cs.client.Walk(basedir)
	for {
		ok := walker.Step()
		if !ok {
			break
		}

		if walkErr := walker.Err(); walkErr != nil {
			if r.IsNotExist(walkErr) {
				debug.Log("ignoring non-existing directory")
				return nil
			}
			err := errors.Wrapf(walkErr, "Walk %v", basedir)
			// Trigger reconnect so the outer retry backend's next
			// attempt gets a fresh connection.
			if r.Config.Reconnect > 0 && isTransientDisconnect(walkErr) {
				debug.Log("List: transient disconnect, triggering reconnect (gen %d): %v", gen, err)
				if _, reconnErr := r.ensureConnected(gen); reconnErr != nil {
					return reconnErr
				}
			}
			return err
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

		rfi := backend.FileInfo{
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
	if r == nil {
		return nil
	}

	r.closed.Store(true)

	r.connMu.Lock()
	cs := r.cs
	r.cs = nil
	r.connMu.Unlock()

	if cs == nil {
		return nil
	}

	cs.close()
	return nil
}

func (r *SFTP) deleteRecursive(ctx context.Context, cs *connState, name string) error {
	entries, err := cs.client.ReadDir(name)
	if err != nil {
		return errors.Wrapf(err, "ReadDir %v", name)
	}

	for _, fi := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		itemName := path.Join(name, fi.Name())
		if fi.IsDir() {
			err := r.deleteRecursive(ctx, cs, itemName)
			if err != nil {
				return err
			}

			err = cs.client.RemoveDirectory(itemName)
			if err != nil {
				return errors.Wrapf(err, "RemoveDirectory %v", itemName)
			}

			continue
		}

		err := cs.client.Remove(itemName)
		if err != nil {
			return errors.Wrapf(err, "Remove %v", itemName)
		}
	}

	return nil
}

// Delete removes all data in the backend.
// Delete is not retried on disconnect because deleteRecursive is not
// idempotent — already-deleted files would cause errors on retry.
func (r *SFTP) Delete(ctx context.Context) error {
	cs, _, err := r.getConn()
	if err != nil {
		return err
	}
	return r.deleteRecursive(ctx, cs, r.p)
}

// Warmup not implemented
func (r *SFTP) Warmup(_ context.Context, _ []backend.Handle) ([]backend.Handle, error) {
	return []backend.Handle{}, nil
}
func (r *SFTP) WarmupWait(_ context.Context, _ []backend.Handle) error { return nil }
