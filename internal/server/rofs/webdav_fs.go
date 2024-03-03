package rofs

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"

	"golang.org/x/net/webdav"
)

// WebDAVFS returns a file system suitable for use with the WebDAV server.
func WebDAVFS(fs *ROFS) webdav.FileSystem {
	return &webDAVFS{FS: fs}
}

// webDAVFS wraps an fs.FS and returns a (read-only) filesystem suitable for use with WebDAV.
type webDAVFS struct {
	fs.FS
}

// ensure that WebDAVFS can be used for webdav.
var _ webdav.FileSystem = &webDAVFS{}

func (*webDAVFS) Mkdir(_ context.Context, name string, _ fs.FileMode) error {
	return &fs.PathError{
		Op:   "Mkdir",
		Path: name,
		Err:  fs.ErrPermission,
	}
}

func (*webDAVFS) RemoveAll(_ context.Context, name string) error {
	return &fs.PathError{
		Op:   "RemoveAll",
		Path: name,
		Err:  fs.ErrPermission,
	}
}

func (*webDAVFS) Rename(_ context.Context, from string, to string) error {
	return &fs.PathError{
		Op:   "Rename",
		Path: from,
		Err:  fs.ErrPermission,
	}
}

func (w *webDAVFS) Open(name string) (fs.File, error) {
	// use relative paths for FS
	name = path.Join(".", name)

	return w.FS.Open(name)
}

func (w *webDAVFS) OpenFile(ctx context.Context, name string, flag int, perm fs.FileMode) (webdav.File, error) {
	// use relative paths for FS
	name = path.Join(".", name)

	if flag != os.O_RDONLY {
		return nil, &fs.PathError{
			Op:   "OpenFile",
			Path: name,
			Err:  fs.ErrPermission,
		}
	}

	f, err := w.FS.Open(name)
	if err != nil {
		return nil, err
	}

	readdirFile, ok := f.(fs.ReadDirFile)
	if !ok {
		readdirFile = nil
	}

	seeker, ok := f.(io.Seeker)
	if !ok {
		seeker = nil
	}

	return &readOnlyFile{File: f, readDirFile: readdirFile, Seeker: seeker}, nil
}

func (w *webDAVFS) Stat(ctx context.Context, name string) (fs.FileInfo, error) {
	// use relative paths for FS
	name = path.Join(".", name)

	f, err := w.FS.Open(name)
	if err != nil {
		return nil, err
	}

	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()

		return nil, err
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	return fi, nil
}

type readOnlyFile struct {
	fs.File
	readDirFile fs.ReadDirFile
	io.Seeker
}

func (f readOnlyFile) Write([]byte) (int, error) {
	return 0, fs.ErrPermission
}

func (f readOnlyFile) Seek(offset int64, whence int) (int64, error) {
	if f.Seeker == nil {
		return 0, fs.ErrInvalid
	}

	return f.Seeker.Seek(offset, whence)
}

func (f readOnlyFile) Readdir(n int) ([]fs.FileInfo, error) {
	if f.readDirFile == nil {
		return nil, fs.ErrInvalid
	}

	entries, err := f.readDirFile.ReadDir(n)
	if err != nil {
		return nil, err
	}

	result := make([]fs.FileInfo, 0, len(entries))

	for _, entry := range entries {
		fi, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("get fileinfo: %w", err)
		}

		result = append(result, fi)
	}

	return result, nil
}
