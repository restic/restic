package fs

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Sftp is the sftp file system
type Sftp struct {
	client *sftp.Client
}

// statically ensure that Sftp implements FS
var _ FS = &Sftp{}

// SftpOptions is the options of Sftp
type SftpOptions struct {
	// Password is the password of user
	Password string

	// KeyFile is the file containing the
	// private key of user
	KeyFile string
}

// NewSftp connect to a sftp server and return the client.
func NewSftp(host string, port int, user string, options SftpOptions) (*Sftp, error) {
	addr := fmt.Sprintf("%s:%d", host, port)

	auth := make([]ssh.AuthMethod, 0)
	if len(options.Password) != 0 {
		passwordAuth := ssh.Password(options.Password)
		auth = append(auth, passwordAuth)
	}

	if len(options.KeyFile) != 0 {
		pemBytes, err := ioutil.ReadFile(options.KeyFile)
		if err != nil {
			return nil, err
		}

		signer, err := ssh.ParsePrivateKey(pemBytes)
		if err != nil {
			return nil, err
		}

		keyAuth := ssh.PublicKeys(signer)
		auth = append(auth, keyAuth)
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: auth,
		// avoid panic
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
	}

	sshClient, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, err
	}

	client, err := sftp.NewClient(sshClient)
	if err != nil {
		return nil, err
	}

	return &Sftp{
		client: client,
	}, nil
}

// VolumeName returns leading volume name. Here only returns "".
func (fs *Sftp) VolumeName(path string) string {
	return ""
}

// Open opens a file for reading.
func (fs Sftp) Open(name string) (File, error) {
	f, err := fs.client.Open(name)
	if err != nil {
		return nil, err
	}
	return &SftpFile{
		File:   f,
		client: fs.client,
	}, nil
}

// OpenFile is the generalized open call; most users will use Open
// or Create instead.  It opens the named file with specified flag
// (O_RDONLY etc.) and perm, (0666 etc.) if applicable.  If successful,
// methods on the returned File can be used for I/O.
// If there is an error, it will be of type *PathError.
// In sftp the perm is invalid.
func (fs Sftp) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	f, err := fs.client.OpenFile(name, flag)
	if err != nil {
		return nil, err
	}
	return &SftpFile{
		File:   f,
		client: fs.client,
	}, nil
}

// Stat returns a FileInfo describing the named file. If there is an error, it
// will be of type *PathError.
func (fs Sftp) Stat(name string) (os.FileInfo, error) {
	return fs.client.Stat(name)
}

// Lstat returns the FileInfo structure describing the named file.
// If the file is a symbolic link, the returned FileInfo
// describes the symbolic link.  Lstat makes no attempt to follow the link.
// If there is an error, it will be of type *PathError.
func (fs Sftp) Lstat(name string) (os.FileInfo, error) {
	return fs.client.Lstat(name)
}

// Join joins any number of path elements into a single path, adding a
// Separator if necessary. Join calls Clean on the result; in particular, all
// empty strings are ignored. On Windows, the result is a UNC path if and only
// if the first path element is a UNC path.
func (fs Sftp) Join(elem ...string) string {
	return fs.client.Join(elem...)
}

// Separator returns the OS and FS dependent separator for dirs/subdirs/files.
func (fs Sftp) Separator() string {
	return string(filepath.Separator)
}

// IsAbs reports whether the path is absolute.
func (fs Sftp) IsAbs(path string) bool {
	return filepath.IsAbs(path)
}

// Abs returns an absolute representation of path. If the path is not absolute
// it will be joined with the current working directory to turn it into an
// absolute path. The absolute path name for a given file is not guaranteed to
// be unique. Abs calls Clean on the result.
func (fs Sftp) Abs(path string) (string, error) {
	return filepath.Abs(path)
}

// Clean returns the cleaned path. For details, see filepath.Clean.
func (fs Sftp) Clean(p string) string {
	return filepath.Clean(p)
}

// Base returns the last element of path.
func (fs Sftp) Base(path string) string {
	return filepath.Base(path)
}

// Dir returns path without the last element.
func (fs Sftp) Dir(path string) string {
	return filepath.Dir(path)
}
