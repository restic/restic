package rofs

import (
	"io"
	"io/fs"
	"path"
	"time"

	"github.com/restic/restic/internal/debug"
)

type MemFile struct {
	Path     string
	FileInfo FileInfo
	Data     []byte
}

// NewMemFile returns a new file.
func NewMemFile(filename string, data []byte, modTime time.Time) MemFile {
	return MemFile{
		Path: filename,
		Data: data,
		FileInfo: FileInfo{
			name:    path.Base(filename),
			size:    int64(len(data)),
			mode:    0644,
			modtime: modTime,
		},
	}
}

func (f MemFile) Open() (fs.File, error) {
	return &openMemFile{
		path:     f.Path,
		fileInfo: f.FileInfo,
		data:     f.Data,
	}, nil
}

func (f MemFile) DirEntry() fs.DirEntry {
	return dirEntry{
		fileInfo: f.FileInfo,
	}
}

// openMemFile is a file that is currently open.
type openMemFile struct {
	path string

	fileInfo FileInfo
	data     []byte

	offset int64
}

// make sure it implements all the necessary interfaces
var _ fs.File = &openMemFile{}
var _ io.Seeker = &openMemFile{}

func (f *openMemFile) Close() error {
	debug.Log("Close(%v)", f.path)

	return nil
}

func (f *openMemFile) Stat() (fs.FileInfo, error) {
	debug.Log("Stat(%v)", f.path)

	return f.fileInfo, nil
}

func (f *openMemFile) Read(p []byte) (int, error) {
	if f.offset >= int64(len(f.data)) {
		debug.Log("Read(%v, %v) -> EOF", f.path, len(p))

		return 0, io.EOF
	}

	if f.offset < 0 {
		debug.Log("Read(%v, %v) -> offset negative", f.path, len(p))

		return 0, &fs.PathError{
			Op:   "read",
			Path: f.path,
			Err:  fs.ErrInvalid,
		}
	}

	n := copy(p, f.data[f.offset:])
	f.offset += int64(n)

	debug.Log("Read(%v, %v) -> %v bytes", f.path, len(p), n)

	return n, nil
}

func (f *openMemFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += f.offset
	case io.SeekEnd:
		offset += int64(len(f.data))
	}

	if offset < 0 || offset > int64(len(f.data)) {
		debug.Log("Seek(%v, %v, %v) -> error invalid offset %v", f.path, offset, whence, offset)

		return 0, &fs.PathError{Op: "seek", Path: f.path, Err: fs.ErrInvalid}
	}

	debug.Log("Seek(%v, %v, %v), new offset %v", f.path, offset, whence, offset)

	f.offset = offset

	return offset, nil
}
