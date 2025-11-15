//go:build darwin || freebsd || linux || windows
// +build darwin freebsd linux windows

package fuse

import (
	"context"
	"io/fs"
	"os"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/restic/restic/internal/data"
)

// This file contains platform-agnostic interfaces and data structures that
// abstract away the github.com/anacrolix/fuse library.

// Node is the interface implemented by all nodes in the filesystem.
type Node interface {
	Attr(ctx context.Context, a *Attr) error
}

// Handle is the interface implemented by all open files and directories.
type Handle interface{}

// NodeForgetter is implemented by nodes that want to be notified when they are no longer needed.
type NodeForgetter interface {
	Node
	Forget()
}

// NodeStringLookuper is implemented by nodes that can look up a child by name.
type NodeStringLookuper interface {
	Node
	Lookup(ctx context.Context, name string) (Node, error)
}

// HandleReadDirAller is implemented by directory handles that can read all entries at once.
type HandleReadDirAller interface {
	Handle
	ReadDirAll(ctx context.Context) ([]Dirent, error)
}

// NodeOpener is implemented by nodes that can be opened.
type NodeOpener interface {
	Node
	Open(ctx context.Context, req *OpenRequest, resp *OpenResponse) (Handle, error)
}

// HandleReader is implemented by file handles that can be read.
type HandleReader interface {
	Handle
	Read(ctx context.Context, req *ReadRequest, resp *ReadResponse) error
}

// NodeListxattrer is implemented by nodes that can list extended attributes.
type NodeListxattrer interface {
	Node
	Listxattr(ctx context.Context, req *ListxattrRequest, resp *ListxattrResponse) error
}

// NodeGetxattrer is implemented by nodes that can get an extended attribute.
type NodeGetxattrer interface {
	Node
	Getxattr(ctx context.Context, req *GetxattrRequest, resp *GetxattrResponse) error
}

// NodeReadlinker is implemented by nodes that can be read as a symlink.
type NodeReadlinker interface {
	Node
	Readlink(ctx context.Context, req *ReadlinkRequest) (string, error)
}

// Attr is a copy of fuse.Attr
type Attr struct {
	Inode     uint64
	Size      uint64
	Blocks    uint64
	Atime     time.Time
	Mtime     time.Time
	Ctime     time.Time
	Mode      os.FileMode
	Nlink     uint32
	Uid       uint32
	Gid       uint32
	Rdev      uint32
	BlockSize uint32
}

// Dirent is a copy of fuse.Dirent
type Dirent struct {
	Inode uint64
	Type  DirentType
	Name  string
	Node  *data.Node
}

// DirentType is a copy of fuse.DirentType
type DirentType uint32

const (
	// These don't quite match os.FileMode; especially there's an
	// explicit unknown, instead of zero value meaning file. They
	// are also not quite syscall.DT_*; nothing says the FUSE
	// protocol follows those, and even if they were, we don't
	// want each fs to fiddle with syscall.

	// The shift by 12 is hardcoded in the FUSE userspace
	// low-level C library, so it's safe here.

	DT_Unknown DirentType = 0
	DT_Socket  DirentType = syscall.S_IFSOCK >> 12
	DT_Link    DirentType = syscall.S_IFLNK >> 12
	DT_File    DirentType = syscall.S_IFREG >> 12
	DT_Block   DirentType = syscall.S_IFBLK >> 12
	DT_Dir     DirentType = syscall.S_IFDIR >> 12
	DT_Char    DirentType = syscall.S_IFCHR >> 12
	DT_FIFO    DirentType = syscall.S_IFIFO >> 12
)

func FileMode2fuseMode(Mode os.FileMode) uint32 {
	fMode := uint32(Mode & os.ModePerm)
	if Mode&os.ModeDir != 0 {
		return fMode | uint32(DT_Dir<<12)
	} else if Mode&os.ModeSymlink != 0 {
		return fMode | uint32(DT_Link<<12)
	} else if Mode&fs.ModeType == 0 {
		return fMode | uint32(DT_File<<12)
	} else if Mode&fs.ModeDevice != 0 {
		return fMode | uint32(DT_Block<<12)
	} else if Mode&fs.ModeCharDevice != 0 {
		return fMode | uint32(DT_Char<<12)
	} else if Mode&fs.ModeNamedPipe != 0 {
		return fMode | uint32(DT_FIFO<<12)
	} else if Mode&fs.ModeSocket != 0 {
		return fMode | uint32(DT_Socket<<12)
	} else {
		return fMode | uint32(DT_Unknown<<12)
	}
}

// OpenRequest is a copy of fuse.OpenRequest
type OpenRequest struct {
	Flags OpenFlags
}

// OpenResponse is a copy of fuse.OpenResponse
type OpenResponse struct {
	Flags OpenResponseFlags
}

// OpenFlags is a copy of fuse.OpenFlags
type OpenFlags uint32

// OpenResponseFlags is a copy of fuse.OpenResponseFlags
type OpenResponseFlags uint32

// ReadRequest is a copy of fuse.ReadRequest
type ReadRequest struct {
	Handle HandleID
	Offset int64
	Size   int
}

// ReadResponse is a copy of fuse.ReadResponse
type ReadResponse struct {
	Data []byte
}

// ReadlinkRequest is a copy of fuse.ReadlinkRequest
type ReadlinkRequest struct{}

// ListxattrRequest is a copy of fuse.ListxattrRequest
type ListxattrRequest struct {
	Size uint32
}

// ListxattrResponse is a copy of fuse.ListxattrResponse
type ListxattrResponse struct {
	Xattr []byte
}

// Append adds a name to the list of extended attributes.
func (r *ListxattrResponse) Append(name string) {
	data := []byte(name)
	data = append(data, 0)
	r.Xattr = append(r.Xattr, data...)
}

// GetxattrRequest is a copy of fuse.GetxattrRequest
type GetxattrRequest struct {
	Name string
	Size uint32
}

// GetxattrResponse is a copy of fuse.GetxattrResponse
type GetxattrResponse struct {
	Xattr []byte
}

// HandleID identifies an open file or directory.
type HandleID uint64

// getNodeForPath resolves a path to an internal fuse.Node.
func getNodeForPath(ctx context.Context, root Node, path string) (Node, error) {
	// Traverse the path from the root
	currentNode := root
	pathParts := splitPath(path) // Helper function to split path

	for _, part := range pathParts {
		lookuper, ok := currentNode.(NodeStringLookuper)
		if !ok {
			return nil, syscall.ENOENT // Not a directory or cannot lookup
		}
		var err error
		currentNode, err = lookuper.Lookup(ctx, part)
		if err != nil {
			return nil, err
		}
	}
	return currentNode, nil
}

// splitPath splits a path into its components, handling leading/trailing slashes.
func splitPath(p string) []string {
	p = path.Clean(p)
	if p == "/" {
		return make([]string, 0)
	}
	return strings.Split(p[1:], "/")
}
