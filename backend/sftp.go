package backend

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/sftp"
)

const (
	tempfileRandomSuffixLength = 10
)

type SFTP struct {
	c   *sftp.Client
	p   string
	ver uint

	cmd *exec.Cmd
}

func start_client(program string, args ...string) (*SFTP, error) {
	// Connect to a remote host and request the sftp subsystem via the 'ssh'
	// command.  This assumes that passwordless login is correctly configured.
	cmd := exec.Command(program, args...)

	// send errors from ssh to stderr
	cmd.Stderr = os.Stderr

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

// OpenSFTP opens an sftp backend. When the command is started via
// exec.Command, it is expected to speak sftp on stdin/stdout. The backend
// is expected at the given path.
func OpenSFTP(dir string, program string, args ...string) (*SFTP, error) {
	sftp, err := start_client(program, args...)
	if err != nil {
		return nil, err
	}

	// test if all necessary dirs and files are there
	items := []string{
		dir,
		filepath.Join(dir, dataPath),
		filepath.Join(dir, snapshotPath),
		filepath.Join(dir, treePath),
		filepath.Join(dir, lockPath),
		filepath.Join(dir, keyPath),
		filepath.Join(dir, tempPath),
	}
	for _, d := range items {
		if _, err := sftp.c.Lstat(d); err != nil {
			return nil, fmt.Errorf("%s does not exist", d)
		}
	}

	// read version file
	f, err := sftp.c.Open(filepath.Join(dir, versionFileName))
	if err != nil {
		return nil, fmt.Errorf("unable to read version file: %v\n", err)
	}

	buf := make([]byte, 100)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return nil, err
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	version, err := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	if err != nil {
		return nil, fmt.Errorf("unable to convert version to integer: %v\n", err)
	}

	// check version
	if version != BackendVersion {
		return nil, fmt.Errorf("wrong version %d", version)
	}

	sftp.p = dir

	return sftp, nil
}

// CreateSFTP creates all the necessary files and directories for a new sftp
// backend at dir.
func CreateSFTP(dir string, program string, args ...string) (*SFTP, error) {
	sftp, err := start_client(program, args...)
	if err != nil {
		return nil, err
	}

	versionFile := filepath.Join(dir, versionFileName)
	dirs := []string{
		dir,
		filepath.Join(dir, dataPath),
		filepath.Join(dir, snapshotPath),
		filepath.Join(dir, treePath),
		filepath.Join(dir, lockPath),
		filepath.Join(dir, keyPath),
		filepath.Join(dir, tempPath),
	}

	// test if version file already exists
	_, err = sftp.c.Lstat(versionFile)
	if err == nil {
		return nil, errors.New("version file already exists")
	}

	// test if directories already exist
	for _, d := range dirs[1:] {
		if _, err := sftp.c.Lstat(d); err == nil {
			return nil, fmt.Errorf("dir %s already exists", d)
		}
	}

	// create paths for data, refs and temp blobs
	for _, d := range dirs {
		err = sftp.mkdirAll(d, dirMode)
		if err != nil {
			return nil, err
		}
	}

	// create version file
	f, err := sftp.c.Create(versionFile)
	if err != nil {
		return nil, err
	}

	_, err = f.Write([]byte(strconv.Itoa(BackendVersion)))
	if err != nil {
		return nil, err
	}

	err = f.Close()
	if err != nil {
		return nil, err
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
	return OpenSFTP(dir, program, args...)
}

// Location returns this backend's location (the directory name).
func (r *SFTP) Location() string {
	return r.p
}

// Return temp directory in correct directory for this backend.
func (r *SFTP) tempFile() (string, *sftp.File, error) {
	// choose random suffix
	buf := make([]byte, tempfileRandomSuffixLength)
	n, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		return "", nil, err
	}

	if n != len(buf) {
		return "", nil, errors.New("unable to generate enough random bytes for temp file")
	}

	// construct tempfile name
	name := filepath.Join(r.p, tempPath, fmt.Sprintf("temp-%s", hex.EncodeToString(buf)))

	// create file in temp dir
	f, err := r.c.Create(name)
	if err != nil {
		return "", nil, err
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
	errMkdirAll := r.mkdirAll(filepath.Dir(dir), dirMode)

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

// Rename temp file to final name according to type and ID.
func (r *SFTP) renameFile(oldname string, t Type, id ID) error {
	newname := r.filename(t, id)

	// create directories if necessary
	if t == Data || t == Tree {
		err := r.mkdirAll(filepath.Dir(newname), dirMode)
		if err != nil {
			return err
		}
	}

	return r.c.Rename(oldname, newname)
}

// Construct directory for given Type.
func (r *SFTP) dirname(t Type, id ID) string {
	var n string
	switch t {
	case Data:
		n = dataPath
		if id != nil {
			n = filepath.Join(dataPath, fmt.Sprintf("%02x", id[0]))
		}
	case Snapshot:
		n = snapshotPath
	case Tree:
		n = treePath
		if id != nil {
			n = filepath.Join(treePath, fmt.Sprintf("%02x", id[0]))
		}
	case Lock:
		n = lockPath
	case Key:
		n = keyPath
	}
	return filepath.Join(r.p, n)
}

type sftpBlob struct {
	f       *sftp.File
	name    string
	h       hash.Hash
	tw      io.Writer
	backend *SFTP
	tpe     Type
	id      ID
}

func (sb *sftpBlob) Close() error {
	err := sb.f.Close()
	if err != nil {
		return err
	}

	// get ID
	sb.id = ID(sb.h.Sum(nil))

	// check for duplicate ID
	res, err := sb.backend.Test(sb.tpe, sb.id)
	if err != nil {
		return fmt.Errorf("testing presence of ID %v failed: %v", sb.id, err)
	}

	if res {
		return ErrAlreadyPresent
	}

	// rename file
	err = sb.backend.renameFile(sb.name, sb.tpe, sb.id)
	if err != nil {
		return err
	}

	return nil
}

func (sb *sftpBlob) Write(p []byte) (int, error) {
	return sb.tw.Write(p)
}

func (sb *sftpBlob) ID() (ID, error) {
	if sb.id == nil {
		return nil, errors.New("blob is not closed, ID unavailable")
	}

	return sb.id, nil
}

// Create creates a new blob of type t. Blob implements io.WriteCloser. Once
// Close() has been called, ID() can be used to retrieve the ID. If the blob is
// already present, Close() returns ErrAlreadyPresent.
func (r *SFTP) Create(t Type) (Blob, error) {
	// TODO: make sure that tempfile is removed upon error

	// create tempfile in backend
	filename, file, err := r.tempFile()
	if err != nil {
		return nil, err
	}

	h := newHash()
	blob := sftpBlob{
		h:       h,
		tw:      io.MultiWriter(h, file),
		f:       file,
		name:    filename,
		backend: r,
		tpe:     t,
	}

	return &blob, nil
}

// Construct path for given Type and ID.
func (r *SFTP) filename(t Type, id ID) string {
	return filepath.Join(r.dirname(t, id), id.String())
}

// Get returns the content stored under the given ID. If the data doesn't match
// the requested ID, ErrWrongData is returned.
func (r *SFTP) Get(t Type, id ID) ([]byte, error) {
	if id == nil {
		return nil, errors.New("unable to load nil ID")
	}

	// try to open file
	file, err := r.c.Open(r.filename(t, id))
	defer func() {
		// TODO: report bug against sftp client, ignore Close() for nil file
		if file != nil {
			file.Close()
		}
	}()
	if err != nil {
		return nil, err
	}

	// read all
	buf, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	// check id
	if !Hash(buf).Equal(id) {
		return nil, ErrWrongData
	}

	return buf, nil
}

// GetReader returns a reader that yields the content stored under the given
// ID. The content is not verified. The reader should be closed after draining
// it.
func (r *SFTP) GetReader(t Type, id ID) (io.ReadCloser, error) {
	if id == nil {
		return nil, errors.New("unable to load nil ID")
	}

	// try to open file
	file, err := r.c.Open(r.filename(t, id))
	if err != nil {
		return nil, err
	}

	return file, nil
}

// Test returns true if a blob of the given type and ID exists in the backend.
func (r *SFTP) Test(t Type, id ID) (bool, error) {
	file, err := r.c.Open(r.filename(t, id))
	defer func() {
		if file != nil {
			file.Close()
		}
	}()

	if err != nil {
		if _, ok := err.(*sftp.StatusError); ok {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// Remove removes the content stored at ID.
func (r *SFTP) Remove(t Type, id ID) error {
	return r.c.Remove(r.filename(t, id))
}

// List lists all objects of a given type.
func (r *SFTP) List(t Type) (IDs, error) {
	list := []os.FileInfo{}
	var err error

	if t == Data || t == Tree {
		// read first level
		basedir := r.dirname(t, nil)

		list1, err := r.c.ReadDir(basedir)
		if err != nil {
			return nil, err
		}

		// read files
		for _, dir := range list1 {
			entries, err := r.c.ReadDir(filepath.Join(basedir, dir.Name()))
			if err != nil {
				return nil, err
			}

			for _, entry := range entries {
				list = append(list, entry)
			}
		}
	} else {
		list, err = r.c.ReadDir(r.dirname(t, nil))
		if err != nil {
			return nil, err
		}
	}

	ids := make(IDs, 0, len(list))
	for _, item := range list {
		id, err := ParseID(item.Name())
		// ignore everything that does not parse as an ID
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// Version returns the version of this local backend.
func (r *SFTP) Version() uint {
	return r.ver
}

// Close closes the sftp connection and terminates the underlying command.
func (s *SFTP) Close() error {
	s.c.Close()
	// TODO: add timeout after which the process is killed
	return s.cmd.Wait()
}
