//go:build darwin || freebsd || linux || windows
// +build darwin freebsd linux windows

package fuse

import (
	"context"
	"io"
	"os"
	"path"
	"sync"
	"time"

	"github.com/restic/restic/internal/debug"
	"golang.org/x/net/webdav"
)

// WebdavFS implements the webdav.FileSystem interface.
type WebdavFS struct {
	root Node
	ctx  context.Context
}

// NewWebdavFS returns a new WebDAV filesystem.
func NewWebdavFS(root Node) *WebdavFS {
	return &WebdavFS{
		root: root,
		ctx:  context.Background(),
	}
}

// Mkdir creates a directory.
func (fs *WebdavFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	debug.Log("WebdavFS.Mkdir(%v, %v)", name, perm)
	return os.ErrPermission
}

// OpenFile opens a file or directory.
func (fs *WebdavFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	debug.Log("WebdavFS.OpenFile(%v, %v, %v)", name, flag, perm)
	if flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		return nil, os.ErrPermission
	}

	node, err := getNodeForPath(fs.ctx, fs.root, name)
	if err != nil {
		debug.Log("WebdavFS.OpenFile(%v): getNodeForPath failed: %v", name, err)
		return nil, err
	}

	var attr Attr
	if err := node.Attr(ctx, &attr); err != nil {
		debug.Log("WebdavFS.OpenFile(%v): node.Attr failed: %v", name, err)
		return nil, err
	}

	var handle Handle
	if !attr.Mode.IsDir() {
		opener, ok := node.(NodeOpener)
		if !ok {
			return nil, os.ErrPermission
		}

		handle, err = opener.Open(ctx, &OpenRequest{}, &OpenResponse{})
		if err != nil {
			debug.Log("WebdavFS.OpenFile(%v): opener.Open failed: %v", name, err)
			return nil, err
		}
	}

	return &webdavFile{
		fs:     fs,
		node:   node,
		handle: handle,
		attr:   attr,
		name:   path.Base(name),
		path:   name,
	}, nil
}

// RemoveAll removes a file or directory.
func (fs *WebdavFS) RemoveAll(ctx context.Context, name string) error {
	debug.Log("WebdavFS.RemoveAll(%v)", name)
	return os.ErrPermission
}

// Rename renames a file or directory.
func (fs *WebdavFS) Rename(ctx context.Context, oldName, newName string) error {
	debug.Log("WebdavFS.Rename(%v, %v)", oldName, newName)
	return os.ErrPermission
}

// Stat returns file information.
func (fs *WebdavFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	debug.Log("WebdavFS.Stat(%v)", name)
	node, err := getNodeForPath(fs.ctx, fs.root, name)
	if err != nil {
		debug.Log("WebdavFS.Stat(%v): getNodeForPath failed: %v", name, err)
		return nil, err
	}

	var attr Attr
	if err := node.Attr(ctx, &attr); err != nil {
		debug.Log("WebdavFS.Stat(%v): node.Attr failed: %v", name, err)
		return nil, err
	}

	return &fileInfo{
		name: path.Base(name),
		attr: attr,
	}, nil
}

// webdavFile implements the webdav.File interface.
type webdavFile struct {
	fs     *WebdavFS
	node   Node
	handle Handle
	attr   Attr
	name   string
	path   string
	mu     sync.Mutex
	offset int64
}

// Close closes the file.
func (f *webdavFile) Close() error {
	debug.Log("webdavFile.Close: %v", f.path)
	return nil
}

// Read reads data from the file.
func (f *webdavFile) Read(p []byte) (int, error) {
	debug.Log("webdavFile.Read: %v", f.path)
	if f.handle == nil {
		return 0, os.ErrInvalid
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	reader, ok := f.handle.(HandleReader)
	if !ok {
		return 0, os.ErrInvalid
	}
	toread := len(p)
	if toread > int(f.attr.Size)-int(f.offset) {
		toread = int(f.attr.Size) - int(f.offset)
	}
	if toread <= 0 {
		return 0, io.EOF
	}

	resp := ReadResponse{Data: p}
	req := &ReadRequest{
		Offset: f.offset,
		Size:   toread,
	}

	err := reader.Read(f.fs.ctx, req, &resp)
	if err != nil {
		debug.Log("webdavFile.Read: %v, err: %v", f.path, err)
		return 0, err
	}

	n := len(resp.Data)
	f.offset += int64(n)
	return n, err
}

func (f *webdavFile) Write(p []byte) (int, error) {
	debug.Log("webdavFile.Write: %v", f.path)
	return 0, os.ErrPermission
}

// Seek sets the file offset.
func (f *webdavFile) Seek(offset int64, whence int) (int64, error) {
	debug.Log("webdavFile.Seek: %v, offset %v, whence %v", f.path, offset, whence)
	if f.handle == nil {
		return 0, os.ErrInvalid
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	switch whence {
	case io.SeekStart:
		f.offset = offset
	case io.SeekCurrent:
		f.offset += offset
	case io.SeekEnd:
		f.offset = int64(f.attr.Size) + offset
	}
	return f.offset, nil
}

// Readdir reads directory entries.
func (f *webdavFile) Readdir(count int) ([]os.FileInfo, error) {
	debug.Log("webdavFile.Readdir: %v", f.path)
	reader, ok := f.node.(HandleReadDirAller)
	if !ok {
		return nil, os.ErrInvalid
	}

	dirents, err := reader.ReadDirAll(f.fs.ctx)
	if err != nil {
		debug.Log("webdavFile.Readdir: %v, err: %v", f.path, err)
		return nil, err
	}

	var infos []os.FileInfo
	for i, dirent := range dirents {
		if i < 2 { //ignore for . and ..
			continue
		}
		if count > 0 && i >= count {
			break
		}

		var attr Attr
		if dirent.Node != nil {
			// Get attributes directly from dirent.Node
			attr = Attr{
				Inode:     dirent.Node.Inode,
				Size:      dirent.Node.Size,
				Mode:      dirent.Node.Mode,
				Mtime:     dirent.Node.ModTime,
				Atime:     dirent.Node.AccessTime,
				Ctime:     dirent.Node.ChangeTime,
				Nlink:     uint32(dirent.Node.Links),
				Uid:       dirent.Node.UID,
				Gid:       dirent.Node.GID,
				BlockSize: 4096, // A reasonable default
			}
		} else {
			// Fallback to lookup if dirent.Node is not available
			node, err := getNodeForPath(f.fs.ctx, f.fs.root, path.Join(f.path, dirent.Name))
			if err != nil {
				debug.Log("lookup for %v failed: %v", dirent.Name, err)
				continue
			}

			if err := node.Attr(f.fs.ctx, &attr); err != nil {
				debug.Log("attr for %v failed: %v", dirent.Name, err)
				continue
			}
		}

		infos = append(infos, &fileInfo{
			name: dirent.Name,
			attr: attr,
		})
	}
	return infos, nil
}

// Stat returns file information.
func (f *webdavFile) Stat() (os.FileInfo, error) {
	debug.Log("webdavFile.Stat: %v,%v,%+v", f.path, f.name, f.attr)
	return &fileInfo{
		name: f.name,
		attr: f.attr,
	}, nil
}

// fileInfo implements os.FileInfo.
type fileInfo struct {
	name string
	attr Attr
}

func (fi *fileInfo) Name() string       { return fi.name }
func (fi *fileInfo) Size() int64        { return int64(fi.attr.Size) }
func (fi *fileInfo) Mode() os.FileMode  { return fi.attr.Mode }
func (fi *fileInfo) ModTime() time.Time { return fi.attr.Mtime }
func (fi *fileInfo) IsDir() bool        { return fi.Mode().IsDir() }
func (fi *fileInfo) Sys() interface{}   { return nil }
