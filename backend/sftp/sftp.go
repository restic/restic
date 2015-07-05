package sftp

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/juju/errors"
	"github.com/pkg/sftp"
	"github.com/restic/restic/backend"
)

const (
	tempfileRandomSuffixLength = 10
)

type SFTP struct {
	c *sftp.Client
	p string

	cmd *exec.Cmd
}

func startClient(program string, args ...string) (*SFTP, error) {
	// Connect to a remote host and request the sftp subsystem via the 'ssh'
	// command.  This assumes that passwordless login is correctly configured.
	cmd := exec.Command(program, args...)

	// send errors from ssh to stderr
	cmd.Stderr = os.Stderr

	// ignore signals sent to the parent (e.g. SIGINT)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	// get stdin and stdout
	wr, err := cmd.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}
	rd, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}

	// start the process
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	// open the SFTP session
	client, err := sftp.NewClientPipe(rd, wr)
	if err != nil {
		log.Fatal(err)
	}

	return &SFTP{c: client, cmd: cmd}, nil
}

// Open opens an sftp backend. When the command is started via
// exec.Command, it is expected to speak sftp on stdin/stdout. The backend
// is expected at the given path.
func Open(dir string, program string, args ...string) (*SFTP, error) {
	sftp, err := startClient(program, args...)
	if err != nil {
		return nil, err
	}

	// test if all necessary dirs and files are there
	items := []string{
		dir,
		filepath.Join(dir, backend.Paths.Data),
		filepath.Join(dir, backend.Paths.Snapshots),
		filepath.Join(dir, backend.Paths.Index),
		filepath.Join(dir, backend.Paths.Locks),
		filepath.Join(dir, backend.Paths.Keys),
		filepath.Join(dir, backend.Paths.Temp),
	}
	for _, d := range items {
		if _, err := sftp.c.Lstat(d); err != nil {
			return nil, fmt.Errorf("%s does not exist", d)
		}
	}

	sftp.p = dir
	return sftp, nil
}

// Create creates all the necessary files and directories for a new sftp
// backend at dir. Afterwards a new config blob should be created.
func Create(dir string, program string, args ...string) (*SFTP, error) {
	sftp, err := startClient(program, args...)
	if err != nil {
		return nil, err
	}

	dirs := []string{
		dir,
		filepath.Join(dir, backend.Paths.Data),
		filepath.Join(dir, backend.Paths.Snapshots),
		filepath.Join(dir, backend.Paths.Index),
		filepath.Join(dir, backend.Paths.Locks),
		filepath.Join(dir, backend.Paths.Keys),
		filepath.Join(dir, backend.Paths.Temp),
	}

	// test if config file already exists
	_, err = sftp.c.Lstat(backend.Paths.Config)
	if err == nil {
		return nil, errors.New("config file already exists")
	}

	// test if directories already exist
	for _, d := range dirs[1:] {
		if _, err := sftp.c.Lstat(d); err == nil {
			return nil, fmt.Errorf("dir %s already exists", d)
		}
	}

	// create paths for data, refs and temp blobs
	for _, d := range dirs {
		err = sftp.mkdirAll(d, backend.Modes.Dir)
		if err != nil {
			return nil, err
		}
	}

	err = sftp.c.Close()
	if err != nil {
		return nil, err
	}

	err = sftp.cmd.Wait()
	if err != nil {
		return nil, err
	}

	// open backend
	return Open(dir, program, args...)
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
		return "", nil, errors.Annotatef(err,
			"unable to read %d random bytes for tempfile name",
			tempfileRandomSuffixLength)
	}

	// construct tempfile name
	name := filepath.Join(r.p, backend.Paths.Temp, "temp-"+hex.EncodeToString(buf))

	// create file in temp dir
	f, err := r.c.Create(name)
	if err != nil {
		return "", nil, errors.Annotatef(err, "creating tempfile %q failed", name)
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

		return fmt.Errorf("mkdirAll(%s): entry exists but is not a directory", dir)
	}

	// create parent directories
	errMkdirAll := r.mkdirAll(filepath.Dir(dir), backend.Modes.Dir)

	// create directory
	errMkdir := r.c.Mkdir(dir)

	// test if directory was created successfully
	fi, err = r.c.Lstat(dir)
	if err != nil {
		// return previous errors
		return fmt.Errorf("mkdirAll(%s): unable to create directories: %v, %v", dir, errMkdirAll, errMkdir)
	}

	if !fi.IsDir() {
		return fmt.Errorf("mkdirAll(%s): entry exists but is not a directory", dir)
	}

	// set mode
	return r.c.Chmod(dir, mode)
}

// Rename temp file to final name according to type and name.
func (r *SFTP) renameFile(oldname string, t backend.Type, name string) error {
	filename := r.filename(t, name)

	// create directories if necessary
	if t == backend.Data {
		err := r.mkdirAll(filepath.Dir(filename), backend.Modes.Dir)
		if err != nil {
			return err
		}
	}

	// test if new file exists
	if _, err := r.c.Lstat(filename); err == nil {
		return fmt.Errorf("Close(): file %v already exists", filename)
	}

	err := r.c.Rename(oldname, filename)
	if err != nil {
		return err
	}

	// set mode to read-only
	fi, err := r.c.Lstat(filename)
	if err != nil {
		return err
	}

	return r.c.Chmod(filename, fi.Mode()&os.FileMode(^uint32(0222)))
}

type sftpBlob struct {
	f        *sftp.File
	tempname string
	size     uint
	closed   bool
	backend  *SFTP
}

func (sb *sftpBlob) Finalize(t backend.Type, name string) error {
	if sb.closed {
		return errors.New("Close() called on closed file")
	}
	sb.closed = true

	err := sb.f.Close()
	if err != nil {
		return fmt.Errorf("sftp: file.Close: %v", err)
	}

	// rename file
	err = sb.backend.renameFile(sb.tempname, t, name)
	if err != nil {
		return fmt.Errorf("sftp: renameFile: %v", err)
	}

	return nil
}

func (sb *sftpBlob) Write(p []byte) (int, error) {
	n, err := sb.f.Write(p)
	sb.size += uint(n)
	return n, err
}

func (sb *sftpBlob) Size() uint {
	return sb.size
}

// Create creates a new Blob. The data is available only after Finalize()
// has been called on the returned Blob.
func (r *SFTP) Create() (backend.Blob, error) {
	// TODO: make sure that tempfile is removed upon error

	// create tempfile in backend
	filename, file, err := r.tempFile()
	if err != nil {
		return nil, errors.Annotate(err, "create tempfile")
	}

	blob := sftpBlob{
		f:        file,
		tempname: filename,
		backend:  r,
	}

	return &blob, nil
}

// Construct path for given backend.Type and name.
func (r *SFTP) filename(t backend.Type, name string) string {
	if t == backend.Config {
		return filepath.Join(r.p, "config")
	}

	return filepath.Join(r.dirname(t, name), name)
}

// Construct directory for given backend.Type.
func (r *SFTP) dirname(t backend.Type, name string) string {
	var n string
	switch t {
	case backend.Data:
		n = backend.Paths.Data
		if len(name) > 2 {
			n = filepath.Join(n, name[:2])
		}
	case backend.Snapshot:
		n = backend.Paths.Snapshots
	case backend.Index:
		n = backend.Paths.Index
	case backend.Lock:
		n = backend.Paths.Locks
	case backend.Key:
		n = backend.Paths.Keys
	}
	return filepath.Join(r.p, n)
}

// Get returns a reader that yields the content stored under the given
// name. The reader should be closed after draining it.
func (r *SFTP) Get(t backend.Type, name string) (io.ReadCloser, error) {
	// try to open file
	file, err := r.c.Open(r.filename(t, name))
	if err != nil {
		return nil, err
	}

	return file, nil
}

// GetReader returns an io.ReadCloser for the Blob with the given name of
// type t at offset and length. If length is 0, the reader reads until EOF.
func (r *SFTP) GetReader(t backend.Type, name string, offset, length uint) (io.ReadCloser, error) {
	f, err := r.c.Open(r.filename(t, name))
	if err != nil {
		return nil, err
	}

	_, err = f.Seek(int64(offset), 0)
	if err != nil {
		return nil, err
	}

	if length == 0 {
		return f, nil
	}

	return backend.LimitReadCloser(f, int64(length)), nil
}

// Test returns true if a blob of the given type and name exists in the backend.
func (r *SFTP) Test(t backend.Type, name string) (bool, error) {
	_, err := r.c.Lstat(r.filename(t, name))
	if err != nil {
		if _, ok := err.(*sftp.StatusError); ok {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// Remove removes the content stored at name.
func (r *SFTP) Remove(t backend.Type, name string) error {
	return r.c.Remove(r.filename(t, name))
}

// List returns a channel that yields all names of blobs of type t. A
// goroutine is started for this. If the channel done is closed, sending
// stops.
func (r *SFTP) List(t backend.Type, done <-chan struct{}) <-chan string {
	ch := make(chan string)

	go func() {
		defer close(ch)

		if t == backend.Data {
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

			sort.Strings(dirs)

			// read files
			for _, dir := range dirs {
				entries, err := r.c.ReadDir(filepath.Join(basedir, dir))
				if err != nil {
					continue
				}

				items := make([]string, 0, len(entries))
				for _, entry := range entries {
					items = append(items, entry.Name())
				}

				sort.Strings(items)

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

			sort.Strings(items)

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

// Close closes the sftp connection and terminates the underlying command.
func (s *SFTP) Close() error {
	if s == nil {
		return nil
	}

	s.c.Close()

	if err := s.cmd.Process.Kill(); err != nil {
		return err
	}

	return s.cmd.Wait()
}
