package webdav

import (
	"context"
	"os"
	"path"
	"strings"

	"github.com/anacrolix/fuse"
	fusefs "github.com/anacrolix/fuse/fs"
	"github.com/restic/restic/internal/debug"
	"golang.org/x/net/webdav"
)

// RepoFileSystem implements a read-only file system on top of a repositoy.
type RepoFileSystem struct {
	ctx  context.Context
	root fuseDir
}

// Mkdir creates a new directory, it is not available for RepoFileSystem.
func (fs *RepoFileSystem) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return webdav.ErrForbidden
}

// RemoveAll recursively removes files and directories, it is not available for RepoFileSystem.
func (fs *RepoFileSystem) RemoveAll(ctx context.Context, name string) error {
	return webdav.ErrForbidden
}

// Rename renames files or directories, it is not available for RepoFileSystem.
func (fs *RepoFileSystem) Rename(ctx context.Context, oldName, newName string) error {
	return webdav.ErrForbidden
}

// open opens a file.
func (fs *RepoFileSystem) open(ctx context.Context, name string) (webdav.File, error) {
	var err error

	name = path.Clean(name)
	parts := strings.Split(name, "/")

	node := fs.root.(fusefs.Node)

	for _, part := range parts {
		if part == "." || part == "" {
			continue
		}

		// if there is a part left, the actual node must be a dir
		nodedir, ok := node.(fuseDir)
		if !ok {
			// didn't get a dir
			return nil, os.ErrNotExist
		}

		node, err = nodedir.Lookup(fs.ctx, part)
		if err != nil {
			if err == fuse.ENOENT {
				return nil, os.ErrNotExist
			}
			return nil, err
		}
	}

	var attr fuse.Attr
	err = node.Attr(fs.ctx, &attr)
	if err != nil {
		return nil, err
	}

	switch {
	case attr.Mode&os.ModeDir != 0: // dir
		return &RepoDir{ctx: fs.ctx, dir: node.(fuseDir), name: name}, nil
	case attr.Mode&os.ModeType == 0: // file
		return &RepoFile{ctx: fs.ctx, file: node.(fuseFile), name: name, size: int64(attr.Size)}, nil
	}

	return &RepoLink{name: name}, nil

}

// OpenFile opens a file for reading.
func (fs *RepoFileSystem) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	debug.Log("Open %v", name)
	if flag != os.O_RDONLY {
		return nil, webdav.ErrForbidden
	}
	return fs.open(ctx, name)
}

// Stat returns information on a file or directory.
func (fs *RepoFileSystem) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	debug.Log("Stat %v", name)

	file, err := fs.open(fs.ctx, name)
	if err != nil {
		return nil, err
	}

	return file.Stat()
}
