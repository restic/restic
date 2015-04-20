package sftp

import (
	"encoding"
	"io"
	"os"
	"path"
	"sync"
	"time"

	"github.com/kr/fs"

	"golang.org/x/crypto/ssh"
)

// New creates a new SFTP client on conn.
func NewClient(conn *ssh.Client) (*Client, error) {
	s, err := conn.NewSession()
	if err != nil {
		return nil, err
	}
	if err := s.RequestSubsystem("sftp"); err != nil {
		return nil, err
	}
	pw, err := s.StdinPipe()
	if err != nil {
		return nil, err
	}
	pr, err := s.StdoutPipe()
	if err != nil {
		return nil, err
	}

	return NewClientPipe(pr, pw)
}

// NewClientPipe creates a new SFTP client given a Reader and a WriteCloser.
// This can be used for connecting to an SFTP server over TCP/TLS or by using
// the system's ssh client program (e.g. via exec.Command).
func NewClientPipe(rd io.Reader, wr io.WriteCloser) (*Client, error) {
	sftp := &Client{
		w: wr,
		r: rd,
	}
	if err := sftp.sendInit(); err != nil {
		return nil, err
	}
	return sftp, sftp.recvVersion()
}

// Client represents an SFTP session on a *ssh.ClientConn SSH connection.
// Multiple Clients can be active on a single SSH connection, and a Client
// may be called concurrently from multiple Goroutines.
//
// Client implements the github.com/kr/fs.FileSystem interface.
type Client struct {
	w      io.WriteCloser
	r      io.Reader
	mu     sync.Mutex // locks mu and seralises commands to the server
	nextid uint32
}

// Close closes the SFTP session.
func (c *Client) Close() error { return c.w.Close() }

// Create creates the named file mode 0666 (before umask), truncating it if
// it already exists. If successful, methods on the returned File can be
// used for I/O; the associated file descriptor has mode O_RDWR.
func (c *Client) Create(path string) (*File, error) {
	return c.open(path, flags(os.O_RDWR|os.O_CREATE|os.O_TRUNC))
}

const sftpProtocolVersion = 3 // http://tools.ietf.org/html/draft-ietf-secsh-filexfer-02

func (c *Client) sendInit() error {
	return sendPacket(c.w, sshFxInitPacket{
		Version: sftpProtocolVersion, // http://tools.ietf.org/html/draft-ietf-secsh-filexfer-02
	})
}

// returns the current value of c.nextid and increments it
// callers is expected to hold c.mu
func (c *Client) nextId() uint32 {
	v := c.nextid
	c.nextid++
	return v
}

func (c *Client) recvVersion() error {
	typ, data, err := recvPacket(c.r)
	if err != nil {
		return err
	}
	if typ != ssh_FXP_VERSION {
		return &unexpectedPacketErr{ssh_FXP_VERSION, typ}
	}

	version, _ := unmarshalUint32(data)
	if version != sftpProtocolVersion {
		return &unexpectedVersionErr{sftpProtocolVersion, version}
	}

	return nil
}

// Walk returns a new Walker rooted at root.
func (c *Client) Walk(root string) *fs.Walker {
	return fs.WalkFS(root, c)
}

// ReadDir reads the directory named by dirname and returns a list of
// directory entries.
func (c *Client) ReadDir(p string) ([]os.FileInfo, error) {
	handle, err := c.opendir(p)
	if err != nil {
		return nil, err
	}
	defer c.close(handle) // this has to defer earlier than the lock below
	var attrs []os.FileInfo
	c.mu.Lock()
	defer c.mu.Unlock()
	var done = false
	for !done {
		id := c.nextId()
		typ, data, err1 := c.sendRequest(sshFxpReaddirPacket{
			Id:     id,
			Handle: handle,
		})
		if err1 != nil {
			err = err1
			done = true
			break
		}
		switch typ {
		case ssh_FXP_NAME:
			sid, data := unmarshalUint32(data)
			if sid != id {
				return nil, &unexpectedIdErr{id, sid}
			}
			count, data := unmarshalUint32(data)
			for i := uint32(0); i < count; i++ {
				var filename string
				filename, data = unmarshalString(data)
				_, data = unmarshalString(data) // discard longname
				var attr *FileStat
				attr, data = unmarshalAttrs(data)
				if filename == "." || filename == ".." {
					continue
				}
				attrs = append(attrs, fileInfoFromStat(attr, path.Base(filename)))
			}
		case ssh_FXP_STATUS:
			// TODO(dfc) scope warning!
			err = eofOrErr(unmarshalStatus(id, data))
			done = true
		default:
			return nil, unimplementedPacketErr(typ)
		}
	}
	if err == io.EOF {
		err = nil
	}
	return attrs, err
}
func (c *Client) opendir(path string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	typ, data, err := c.sendRequest(sshFxpOpendirPacket{
		Id:   id,
		Path: path,
	})
	if err != nil {
		return "", err
	}
	switch typ {
	case ssh_FXP_HANDLE:
		sid, data := unmarshalUint32(data)
		if sid != id {
			return "", &unexpectedIdErr{id, sid}
		}
		handle, _ := unmarshalString(data)
		return handle, nil
	case ssh_FXP_STATUS:
		return "", unmarshalStatus(id, data)
	default:
		return "", unimplementedPacketErr(typ)
	}
}

func (c *Client) Lstat(p string) (os.FileInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	typ, data, err := c.sendRequest(sshFxpLstatPacket{
		Id:   id,
		Path: p,
	})
	if err != nil {
		return nil, err
	}
	switch typ {
	case ssh_FXP_ATTRS:
		sid, data := unmarshalUint32(data)
		if sid != id {
			return nil, &unexpectedIdErr{id, sid}
		}
		attr, _ := unmarshalAttrs(data)
		return fileInfoFromStat(attr, path.Base(p)), nil
	case ssh_FXP_STATUS:
		return nil, unmarshalStatus(id, data)
	default:
		return nil, unimplementedPacketErr(typ)
	}
}

// ReadLink reads the target of a symbolic link.
func (c *Client) ReadLink(p string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	typ, data, err := c.sendRequest(sshFxpReadlinkPacket{
		Id:   id,
		Path: p,
	})
	if err != nil {
		return "", err
	}
	switch typ {
	case ssh_FXP_NAME:
		sid, data := unmarshalUint32(data)
		if sid != id {
			return "", &unexpectedIdErr{id, sid}
		}
		count, data := unmarshalUint32(data)
		if count != 1 {
			return "", unexpectedCount(1, count)
		}
		filename, _ := unmarshalString(data) // ignore dummy attributes
		return filename, nil
	case ssh_FXP_STATUS:
		return "", unmarshalStatus(id, data)
	default:
		return "", unimplementedPacketErr(typ)
	}
}

// setstat is a convience wrapper to allow for changing of various parts of the file descriptor.
func (c *Client) setstat(path string, flags uint32, attrs interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	typ, data, err := c.sendRequest(sshFxpSetstatPacket{
		Id:    id,
		Path:  path,
		Flags: flags,
		Attrs: attrs,
	})
	if err != nil {
		return err
	}
	switch typ {
	case ssh_FXP_STATUS:
		return okOrErr(unmarshalStatus(id, data))
	default:
		return unimplementedPacketErr(typ)
	}
}

// Chtimes changes the access and modification times of the named file.
func (c *Client) Chtimes(path string, atime time.Time, mtime time.Time) error {
	type times struct {
		Atime uint32
		Mtime uint32
	}
	attrs := times{uint32(atime.Unix()), uint32(mtime.Unix())}
	return c.setstat(path, ssh_FILEXFER_ATTR_ACMODTIME, attrs)
}

// Chown changes the user and group owners of the named file.
func (c *Client) Chown(path string, uid, gid int) error {
	type owner struct {
		Uid uint32
		Gid uint32
	}
	attrs := owner{uint32(uid), uint32(gid)}
	return c.setstat(path, ssh_FILEXFER_ATTR_UIDGID, attrs)
}

// Chmod changes the permissions of the named file.
func (c *Client) Chmod(path string, mode os.FileMode) error {
	return c.setstat(path, ssh_FILEXFER_ATTR_PERMISSIONS, uint32(mode))
}

// Truncate sets the size of the named file. Although it may be safely assumed
// that if the size is less than its current size it will be truncated to fit,
// the SFTP protocol does not specify what behavior the server should do when setting
// size greater than the current size.
func (c *Client) Truncate(path string, size int64) error {
	return c.setstat(path, ssh_FILEXFER_ATTR_SIZE, uint64(size))
}

// Open opens the named file for reading. If successful, methods on the
// returned file can be used for reading; the associated file descriptor
// has mode O_RDONLY.
func (c *Client) Open(path string) (*File, error) {
	return c.open(path, flags(os.O_RDONLY))
}

// OpenFile is the generalized open call; most users will use Open or
// Create instead. It opens the named file with specified flag (O_RDONLY
// etc.). If successful, methods on the returned File can be used for I/O.
func (c *Client) OpenFile(path string, f int) (*File, error) {
	return c.open(path, flags(f))
}

func (c *Client) open(path string, pflags uint32) (*File, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	typ, data, err := c.sendRequest(sshFxpOpenPacket{
		Id:     id,
		Path:   path,
		Pflags: pflags,
	})
	if err != nil {
		return nil, err
	}
	switch typ {
	case ssh_FXP_HANDLE:
		sid, data := unmarshalUint32(data)
		if sid != id {
			return nil, &unexpectedIdErr{id, sid}
		}
		handle, _ := unmarshalString(data)
		return &File{c: c, path: path, handle: handle}, nil
	case ssh_FXP_STATUS:
		return nil, unmarshalStatus(id, data)
	default:
		return nil, unimplementedPacketErr(typ)
	}
}

// readAt reads len(buf) bytes from the remote file indicated by handle starting
// from offset.
func (c *Client) readAt(handle string, offset uint64, buf []byte) (uint32, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	typ, data, err := c.sendRequest(sshFxpReadPacket{
		Id:     id,
		Handle: handle,
		Offset: offset,
		Len:    uint32(len(buf)),
	})
	if err != nil {
		return 0, err
	}
	switch typ {
	case ssh_FXP_DATA:
		sid, data := unmarshalUint32(data)
		if sid != id {
			return 0, &unexpectedIdErr{id, sid}
		}
		l, data := unmarshalUint32(data)
		n := copy(buf, data[:l])
		return uint32(n), nil
	case ssh_FXP_STATUS:
		return 0, eofOrErr(unmarshalStatus(id, data))
	default:
		return 0, unimplementedPacketErr(typ)
	}
}

// close closes a handle handle previously returned in the response
// to SSH_FXP_OPEN or SSH_FXP_OPENDIR. The handle becomes invalid
// immediately after this request has been sent.
func (c *Client) close(handle string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	typ, data, err := c.sendRequest(sshFxpClosePacket{
		Id:     id,
		Handle: handle,
	})
	if err != nil {
		return err
	}
	switch typ {
	case ssh_FXP_STATUS:
		return okOrErr(unmarshalStatus(id, data))
	default:
		return unimplementedPacketErr(typ)
	}
}

func (c *Client) fstat(handle string) (*FileStat, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	typ, data, err := c.sendRequest(sshFxpFstatPacket{
		Id:     id,
		Handle: handle,
	})
	if err != nil {
		return nil, err
	}
	switch typ {
	case ssh_FXP_ATTRS:
		sid, data := unmarshalUint32(data)
		if sid != id {
			return nil, &unexpectedIdErr{id, sid}
		}
		attr, _ := unmarshalAttrs(data)
		return attr, nil
	case ssh_FXP_STATUS:
		return nil, unmarshalStatus(id, data)
	default:
		return nil, unimplementedPacketErr(typ)
	}
}

// Join joins any number of path elements into a single path, adding a
// separating slash if necessary. The result is Cleaned; in particular, all
// empty strings are ignored.
func (c *Client) Join(elem ...string) string { return path.Join(elem...) }

// Remove removes the specified file or directory. An error will be returned if no
// file or directory with the specified path exists, or if the specified directory
// is not empty.
func (c *Client) Remove(path string) error {
	err := c.removeFile(path)
	if status, ok := err.(*StatusError); ok && status.Code == ssh_FX_FAILURE {
		err = c.removeDirectory(path)
	}
	return err
}

func (c *Client) removeFile(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	typ, data, err := c.sendRequest(sshFxpRemovePacket{
		Id:       id,
		Filename: path,
	})
	if err != nil {
		return err
	}
	switch typ {
	case ssh_FXP_STATUS:
		return okOrErr(unmarshalStatus(id, data))
	default:
		return unimplementedPacketErr(typ)
	}
}

func (c *Client) removeDirectory(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	typ, data, err := c.sendRequest(sshFxpRmdirPacket{
		Id:   id,
		Path: path,
	})
	if err != nil {
		return err
	}
	switch typ {
	case ssh_FXP_STATUS:
		return okOrErr(unmarshalStatus(id, data))
	default:
		return unimplementedPacketErr(typ)
	}
}

// Rename renames a file.
func (c *Client) Rename(oldname, newname string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	typ, data, err := c.sendRequest(sshFxpRenamePacket{
		Id:      id,
		Oldpath: oldname,
		Newpath: newname,
	})
	if err != nil {
		return err
	}
	switch typ {
	case ssh_FXP_STATUS:
		return okOrErr(unmarshalStatus(id, data))
	default:
		return unimplementedPacketErr(typ)
	}
}

func (c *Client) sendRequest(p encoding.BinaryMarshaler) (byte, []byte, error) {
	if err := sendPacket(c.w, p); err != nil {
		return 0, nil, err
	}
	return recvPacket(c.r)
}

// writeAt writes len(buf) bytes from the remote file indicated by handle starting
// from offset.
func (c *Client) writeAt(handle string, offset uint64, buf []byte) (uint32, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	typ, data, err := c.sendRequest(sshFxpWritePacket{
		Id:     id,
		Handle: handle,
		Offset: offset,
		Length: uint32(len(buf)),
		Data:   buf,
	})
	if err != nil {
		return 0, err
	}
	switch typ {
	case ssh_FXP_STATUS:
		if err := okOrErr(unmarshalStatus(id, data)); err != nil {
			return 0, err
		}
		return uint32(len(buf)), nil
	default:
		return 0, unimplementedPacketErr(typ)
	}
}

// Creates the specified directory. An error will be returned if a file or
// directory with the specified path already exists, or if the directory's
// parent folder does not exist (the method cannot create complete paths).
func (c *Client) Mkdir(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	typ, data, err := c.sendRequest(sshFxpMkdirPacket{
		Id:   id,
		Path: path,
	})
	if err != nil {
		return err
	}
	switch typ {
	case ssh_FXP_STATUS:
		return okOrErr(unmarshalStatus(id, data))
	default:
		return unimplementedPacketErr(typ)
	}
}

// File represents a remote file.
type File struct {
	c      *Client
	path   string
	handle string
	offset uint64 // current offset within remote file
}

// Close closes the File, rendering it unusable for I/O. It returns an
// error, if any.
func (f *File) Close() error {
	return f.c.close(f.handle)
}

// Read reads up to len(b) bytes from the File. It returns the number of
// bytes read and an error, if any. EOF is signaled by a zero count with
// err set to io.EOF.
func (f *File) Read(b []byte) (int, error) {
	var read int
	for len(b) > 0 {
		n, err := f.c.readAt(f.handle, f.offset, b[:min(len(b), maxWritePacket)])
		f.offset += uint64(n)
		read += int(n)
		if err != nil {
			return read, err
		}
		b = b[n:]
	}
	return read, nil
}

// Stat returns the FileInfo structure describing file. If there is an
// error.
func (f *File) Stat() (os.FileInfo, error) {
	fs, err := f.c.fstat(f.handle)
	if err != nil {
		return nil, err
	}
	return fileInfoFromStat(fs, path.Base(f.path)), nil
}

// clamp writes to less than 32k
const maxWritePacket = 1 << 15

// Write writes len(b) bytes to the File. It returns the number of bytes
// written and an error, if any. Write returns a non-nil error when n !=
// len(b).
func (f *File) Write(b []byte) (int, error) {
	var written int
	for len(b) > 0 {
		n, err := f.c.writeAt(f.handle, f.offset, b[:min(len(b), maxWritePacket)])
		f.offset += uint64(n)
		written += int(n)
		if err != nil {
			return written, err
		}
		b = b[n:]
	}
	return written, nil
}

// Seek implements io.Seeker by setting the client offset for the next Read or
// Write. It returns the next offset read. Seeking before or after the end of
// the file is undefined. Seeking relative to the end calls Stat.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case os.SEEK_SET:
		f.offset = uint64(offset)
	case os.SEEK_CUR:
		f.offset = uint64(int64(f.offset) + offset)
	case os.SEEK_END:
		fi, err := f.Stat()
		if err != nil {
			return int64(f.offset), err
		}
		f.offset = uint64(fi.Size() + offset)
	default:
		return int64(f.offset), unimplementedSeekWhence(whence)
	}
	return int64(f.offset), nil
}

// Chown changes the uid/gid of the current file.
func (f *File) Chown(uid, gid int) error {
	return f.c.Chown(f.path, uid, gid)
}

// Chmod changes the permissions of the current file.
func (f *File) Chmod(mode os.FileMode) error {
	return f.c.Chmod(f.path, mode)
}

// Truncate sets the size of the current file. Although it may be safely assumed
// that if the size is less than its current size it will be truncated to fit,
// the SFTP protocol does not specify what behavior the server should do when setting
// size greater than the current size.
func (f *File) Truncate(size int64) error {
	return f.c.Truncate(f.path, size)
}

func min(a, b int) int {
	if a > b {
		return b
	}
	return a
}

// okOrErr returns nil if Err.Code is SSH_FX_OK, otherwise it returns the error.
func okOrErr(err error) error {
	if err, ok := err.(*StatusError); ok && err.Code == ssh_FX_OK {
		return nil
	}
	return err
}

func eofOrErr(err error) error {
	if err, ok := err.(*StatusError); ok && err.Code == ssh_FX_EOF {
		return io.EOF
	}
	return err
}

func unmarshalStatus(id uint32, data []byte) error {
	sid, data := unmarshalUint32(data)
	if sid != id {
		return &unexpectedIdErr{id, sid}
	}
	code, data := unmarshalUint32(data)
	msg, data := unmarshalString(data)
	lang, _ := unmarshalString(data)
	return &StatusError{
		Code: code,
		msg:  msg,
		lang: lang,
	}
}

// flags converts the flags passed to OpenFile into ssh flags.
// Unsupported flags are ignored.
func flags(f int) uint32 {
	var out uint32
	switch f & os.O_WRONLY {
	case os.O_WRONLY:
		out |= ssh_FXF_WRITE
	case os.O_RDONLY:
		out |= ssh_FXF_READ
	}
	if f&os.O_RDWR == os.O_RDWR {
		out |= ssh_FXF_READ | ssh_FXF_WRITE
	}
	if f&os.O_APPEND == os.O_APPEND {
		out |= ssh_FXF_APPEND
	}
	if f&os.O_CREATE == os.O_CREATE {
		out |= ssh_FXF_CREAT
	}
	if f&os.O_TRUNC == os.O_TRUNC {
		out |= ssh_FXF_TRUNC
	}
	if f&os.O_EXCL == os.O_EXCL {
		out |= ssh_FXF_EXCL
	}
	return out
}
