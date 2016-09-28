package sftp

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"restic"
	"strings"
	"time"

	"restic/errors"

	"restic/backend"
	"restic/debug"

	"github.com/pkg/sftp"
)

const (
	tempfileRandomSuffixLength = 10
)

// SFTP is a backend in a directory accessed via SFTP.
type SFTP struct {
	c *sftp.Client
	p string

	cmd    *exec.Cmd
	result <-chan error
}

var _ restic.Backend = &SFTP{}

func startClient(program string, args ...string) (*SFTP, error) {
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

	// ignore signals sent to the parent (e.g. SIGINT)
	cmd.SysProcAttr = ignoreSigIntProcAttr()

	// get stdin and stdout
	wr, err := cmd.StdinPipe()
	if err != nil {
		return nil, errors.Wrap(err, "cmd.StdinPipe")
	}
	rd, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, "cmd.StdoutPipe")
	}

	// start the process
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "cmd.Start")
	}

	// wait in a different goroutine
	ch := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		debug.Log("ssh command exited, err %v", err)
		ch <- errors.Wrap(err, "cmd.Wait")
	}()

	// open the SFTP session
	client, err := sftp.NewClientPipe(rd, wr)
	if err != nil {
		return nil, errors.Errorf("unable to start the sftp session, error: %v", err)
	}

	return &SFTP{c: client, cmd: cmd, result: ch}, nil
}

func paths(dir string) []string {
	return []string{
		dir,
		Join(dir, backend.Paths.Data),
		Join(dir, backend.Paths.Snapshots),
		Join(dir, backend.Paths.Index),
		Join(dir, backend.Paths.Locks),
		Join(dir, backend.Paths.Keys),
		Join(dir, backend.Paths.Temp),
	}
}

// clientError returns an error if the client has exited. Otherwise, nil is
// returned immediately.
func (r *SFTP) clientError() error {
	select {
	case err := <-r.result:
		debug.Log("client has exited with err %v", err)
		return err
	default:
	}

	return nil
}

// Open opens an sftp backend. When the command is started via
// exec.Command, it is expected to speak sftp on stdin/stdout. The backend
// is expected at the given path. `dir` must be delimited by forward slashes
// ("/"), which is required by sftp.
func Open(dir string, program string, args ...string) (*SFTP, error) {
	debug.Log("open backend with program %v, %v at %v", program, args, dir)
	sftp, err := startClient(program, args...)
	if err != nil {
		debug.Log("unable to start program: %v", err)
		return nil, err
	}

	// test if all necessary dirs and files are there
	for _, d := range paths(dir) {
		if _, err := sftp.c.Lstat(d); err != nil {
			return nil, errors.Errorf("%s does not exist", d)
		}
	}

	sftp.p = dir
	return sftp, nil
}

func buildSSHCommand(cfg Config) []string {
	hostport := strings.Split(cfg.Host, ":")
	args := []string{hostport[0]}
	if len(hostport) > 1 {
		args = append(args, "-p", hostport[1])
	}
	if cfg.User != "" {
		args = append(args, "-l")
		args = append(args, cfg.User)
	}
	args = append(args, "-s")
	args = append(args, "sftp")
	return args
}

// OpenWithConfig opens an sftp backend as described by the config by running
// "ssh" with the appropiate arguments.
func OpenWithConfig(cfg Config) (*SFTP, error) {
	debug.Log("open with config %v", cfg)
	return Open(cfg.Dir, "ssh", buildSSHCommand(cfg)...)
}

// Create creates all the necessary files and directories for a new sftp
// backend at dir. Afterwards a new config blob should be created. `dir` must
// be delimited by forward slashes ("/"), which is required by sftp.
func Create(dir string, program string, args ...string) (*SFTP, error) {
	debug.Log("%v %v", program, args)
	sftp, err := startClient(program, args...)
	if err != nil {
		return nil, err
	}

	// test if config file already exists
	_, err = sftp.c.Lstat(Join(dir, backend.Paths.Config))
	if err == nil {
		return nil, errors.New("config file already exists")
	}

	// create paths for data, refs and temp blobs
	for _, d := range paths(dir) {
		err = sftp.mkdirAll(d, backend.Modes.Dir)
		if err != nil {
			return nil, err
		}
	}

	err = sftp.Close()
	if err != nil {
		return nil, errors.Wrap(err, "Close")
	}

	// open backend
	return Open(dir, program, args...)
}

// CreateWithConfig creates an sftp backend as described by the config by running
// "ssh" with the appropiate arguments.
func CreateWithConfig(cfg Config) (*SFTP, error) {
	debug.Log("config %v", cfg)
	return Create(cfg.Dir, "ssh", buildSSHCommand(cfg)...)
}

// Location returns this backend's location (the directory name).
func (r *SFTP) Location() string {
	return r.p
}

// Return temp directory in correct directory for this backend.
func (r *SFTP) tempFile() (string, *sftp.File, error) {
	// choose random suffix
	buf := make([]byte, tempfileRandomSuffixLength)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		return "", nil, errors.Errorf("unable to read %d random bytes for tempfile name: %v",
			tempfileRandomSuffixLength, err)
	}

	// construct tempfile name
	name := Join(r.p, backend.Paths.Temp, "temp-"+hex.EncodeToString(buf))

	// create file in temp dir
	f, err := r.c.Create(name)
	if err != nil {
		return "", nil, errors.Errorf("creating tempfile %q failed: %v", name, err)
	}

	return name, f, nil
}

func (r *SFTP) mkdirAll(dir string, mode os.FileMode) error {
	// check if directory already exists
	fi, err := r.c.Lstat(dir)
	if err == nil {
		if fi.IsDir() {
			return nil
		}

		return errors.Errorf("mkdirAll(%s): entry exists but is not a directory", dir)
	}

	// create parent directories
	errMkdirAll := r.mkdirAll(path.Dir(dir), backend.Modes.Dir)

	// create directory
	errMkdir := r.c.Mkdir(dir)

	// test if directory was created successfully
	fi, err = r.c.Lstat(dir)
	if err != nil {
		// return previous errors
		return errors.Errorf("mkdirAll(%s): unable to create directories: %v, %v", dir, errMkdirAll, errMkdir)
	}

	if !fi.IsDir() {
		return errors.Errorf("mkdirAll(%s): entry exists but is not a directory", dir)
	}

	// set mode
	return r.c.Chmod(dir, mode)
}

// Rename temp file to final name according to type and name.
func (r *SFTP) renameFile(oldname string, t restic.FileType, name string) error {
	filename := r.filename(t, name)

	// create directories if necessary
	if t == restic.DataFile {
		err := r.mkdirAll(path.Dir(filename), backend.Modes.Dir)
		if err != nil {
			return err
		}
	}

	// test if new file exists
	if _, err := r.c.Lstat(filename); err == nil {
		return errors.Errorf("Close(): file %v already exists", filename)
	}

	err := r.c.Rename(oldname, filename)
	if err != nil {
		return errors.Wrap(err, "Rename")
	}

	// set mode to read-only
	fi, err := r.c.Lstat(filename)
	if err != nil {
		return errors.Wrap(err, "Lstat")
	}

	err = r.c.Chmod(filename, fi.Mode()&os.FileMode(^uint32(0222)))
	return errors.Wrap(err, "Chmod")
}

// Join joins the given paths and cleans them afterwards. This always uses
// forward slashes, which is required by sftp.
func Join(parts ...string) string {
	return path.Clean(path.Join(parts...))
}

// Construct path for given restic.Type and name.
func (r *SFTP) filename(t restic.FileType, name string) string {
	if t == restic.ConfigFile {
		return Join(r.p, "config")
	}

	return Join(r.dirname(t, name), name)
}

// Construct directory for given backend.Type.
func (r *SFTP) dirname(t restic.FileType, name string) string {
	var n string
	switch t {
	case restic.DataFile:
		n = backend.Paths.Data
		if len(name) > 2 {
			n = Join(n, name[:2])
		}
	case restic.SnapshotFile:
		n = backend.Paths.Snapshots
	case restic.IndexFile:
		n = backend.Paths.Index
	case restic.LockFile:
		n = backend.Paths.Locks
	case restic.KeyFile:
		n = backend.Paths.Keys
	}
	return Join(r.p, n)
}

// Load returns the data stored in the backend for h at the given offset
// and saves it in p. Load has the same semantics as io.ReaderAt.
func (r *SFTP) Load(h restic.Handle, p []byte, off int64) (n int, err error) {
	debug.Log("load %v, %d bytes, offset %v", h, len(p), off)
	if err := r.clientError(); err != nil {
		return 0, err
	}

	if err := h.Valid(); err != nil {
		return 0, err
	}

	f, err := r.c.Open(r.filename(h.Type, h.Name))
	if err != nil {
		return 0, errors.Wrap(err, "Open")
	}

	defer func() {
		e := f.Close()
		if err == nil && e != nil {
			err = errors.Wrap(e, "Close")
		}
	}()

	switch {
	case off > 0:
		_, err = f.Seek(off, 0)
	case off < 0:
		_, err = f.Seek(off, 2)
	}

	if err != nil {
		return 0, errors.Wrap(err, "Seek")
	}

	return io.ReadFull(f, p)
}

// Save stores data in the backend at the handle.
func (r *SFTP) Save(h restic.Handle, p []byte) (err error) {
	debug.Log("save %v bytes to %v", h, len(p))
	if err := r.clientError(); err != nil {
		return err
	}

	if err := h.Valid(); err != nil {
		return err
	}

	filename, tmpfile, err := r.tempFile()
	if err != nil {
		return err
	}

	debug.Log("save %v (%d bytes) to %v", h, len(p), filename)

	n, err := tmpfile.Write(p)
	if err != nil {
		return errors.Wrap(err, "Write")
	}

	if n != len(p) {
		return errors.New("not all bytes writen")
	}

	err = tmpfile.Close()
	if err != nil {
		return errors.Wrap(err, "Close")
	}

	err = r.renameFile(filename, h.Type, h.Name)
	debug.Log("save %v: rename %v: %v",
		h, path.Base(filename), err)
	return err
}

// Stat returns information about a blob.
func (r *SFTP) Stat(h restic.Handle) (restic.FileInfo, error) {
	debug.Log("stat %v", h)
	if err := r.clientError(); err != nil {
		return restic.FileInfo{}, err
	}

	if err := h.Valid(); err != nil {
		return restic.FileInfo{}, err
	}

	fi, err := r.c.Lstat(r.filename(h.Type, h.Name))
	if err != nil {
		return restic.FileInfo{}, errors.Wrap(err, "Lstat")
	}

	return restic.FileInfo{Size: fi.Size()}, nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (r *SFTP) Test(t restic.FileType, name string) (bool, error) {
	debug.Log("type %v, name %v", t, name)
	if err := r.clientError(); err != nil {
		return false, err
	}

	_, err := r.c.Lstat(r.filename(t, name))
	if os.IsNotExist(errors.Cause(err)) {
		return false, nil
	}

	if err != nil {
		return false, errors.Wrap(err, "Lstat")
	}

	return true, nil
}

// Remove removes the content stored at name.
func (r *SFTP) Remove(t restic.FileType, name string) error {
	debug.Log("type %v, name %v", t, name)
	if err := r.clientError(); err != nil {
		return err
	}

	return r.c.Remove(r.filename(t, name))
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (r *SFTP) List(t restic.FileType, done <-chan struct{}) <-chan string {
	debug.Log("list all %v", t)
	ch := make(chan string)

	go func() {
		defer close(ch)

		if t == restic.DataFile {
			// read first level
			basedir := r.dirname(t, "")

			list1, err := r.c.ReadDir(basedir)
			if err != nil {
				return
			}

			dirs := make([]string, 0, len(list1))
			for _, d := range list1 {
				dirs = append(dirs, d.Name())
			}

			// read files
			for _, dir := range dirs {
				entries, err := r.c.ReadDir(Join(basedir, dir))
				if err != nil {
					continue
				}

				items := make([]string, 0, len(entries))
				for _, entry := range entries {
					items = append(items, entry.Name())
				}

				for _, file := range items {
					select {
					case ch <- file:
					case <-done:
						return
					}
				}
			}
		} else {
			entries, err := r.c.ReadDir(r.dirname(t, ""))
			if err != nil {
				return
			}

			items := make([]string, 0, len(entries))
			for _, entry := range entries {
				items = append(items, entry.Name())
			}

			for _, file := range items {
				select {
				case ch <- file:
				case <-done:
					return
				}
			}
		}
	}()

	return ch

}

var closeTimeout = 2 * time.Second

// Close closes the sftp connection and terminates the underlying command.
func (r *SFTP) Close() error {
	debug.Log("")
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
