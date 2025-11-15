//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fuse

import (
	"context"
	"syscall"

	"github.com/anacrolix/fuse"
	"github.com/anacrolix/fuse/fs"
	"github.com/restic/restic/internal/debug"
)

// FS is a FUSE filesystem that provides access to a repository.
// It is the bridge between our internal FUSE implementation and the anacrolix/fuse library.
type FS struct {
	root Node
}

// NewFS returns a new FUSE filesystem.
func NewFS(root Node) *FS {
	return &FS{
		root: root,
	}
}

// Root returns the root node of the filesystem.
func (f *FS) Root() (fs.Node, error) {
	return &nodeWrapper{node: f.root}, nil
}

// nodeWrapper wraps our internal Node interface to satisfy the anacrolix/fuse/fs.Node interface.
type nodeWrapper struct {
	node Node
}
type NodeOpenerWrapper struct {
	nodeWrapper
}

func (w *nodeWrapper) Attr(ctx context.Context, attr *fuse.Attr) error {
	debug.Log("bridge: nodeWrapper.Attr")
	// Convert our Attr to fuse.Attr
	var ourAttr Attr
	err := w.node.Attr(ctx, &ourAttr)
	if err != nil {
		return err
	}

	attr.Inode = ourAttr.Inode
	attr.Size = ourAttr.Size
	attr.Blocks = ourAttr.Blocks
	attr.Atime = ourAttr.Atime
	attr.Mtime = ourAttr.Mtime
	attr.Ctime = ourAttr.Ctime
	attr.Mode = ourAttr.Mode
	attr.Nlink = ourAttr.Nlink
	attr.Uid = ourAttr.Uid
	attr.Gid = ourAttr.Gid
	attr.Rdev = ourAttr.Rdev
	attr.BlockSize = ourAttr.BlockSize

	return nil
}

// Make sure nodeWrapper implements all the interfaces we need.
var _ fs.Node = (*nodeWrapper)(nil)
var _ fs.NodeStringLookuper = (*nodeWrapper)(nil)
var _ fs.NodeForgetter = (*nodeWrapper)(nil)
var _ fs.NodeReadlinker = (*nodeWrapper)(nil)

var _ fs.NodeGetxattrer = (*nodeWrapper)(nil)
var _ fs.NodeListxattrer = (*nodeWrapper)(nil)

var _ fs.NodeOpener = (*NodeOpenerWrapper)(nil)

func (w *nodeWrapper) Lookup(ctx context.Context, name string) (fs.Node, error) {
	debug.Log("bridge: nodeWrapper.Lookup for %s", name)
	lookuper, ok := w.node.(NodeStringLookuper)
	if !ok {
		return nil, syscall.ENOSYS
	}

	node, err := lookuper.Lookup(ctx, name)
	if err != nil {
		return nil, err
	}

	if _, ok := node.(NodeOpener); ok {
		return &NodeOpenerWrapper{nodeWrapper{node: node}}, nil
	} else {
		return &nodeWrapper{node: node}, nil
	}
}

func (w *nodeWrapper) Forget() {
	debug.Log("bridge: nodeWrapper.Forget")
	forgetter, ok := w.node.(NodeForgetter)
	if ok {
		forgetter.Forget()
	}
}

func (w *nodeWrapper) Readlink(ctx context.Context, req *fuse.ReadlinkRequest) (string, error) {
	debug.Log("bridge: nodeWrapper.Readlink")
	readlinker, ok := w.node.(NodeReadlinker)
	if !ok {
		return "", syscall.ENOSYS
	}

	// Our ReadlinkRequest is empty, so we can just pass a new one.
	return readlinker.Readlink(ctx, &ReadlinkRequest{})
}

// handleWrapper wraps our internal Handle interface to satisfy the anacrolix/fuse/fs.Handle interface.
// only file can be open, directory is not supported by the backend
type handleWrapper struct {
	handle Handle
}

var _ fs.Handle = (*handleWrapper)(nil)
var _ fs.HandleReader = (*handleWrapper)(nil)

func (w *NodeOpenerWrapper) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	debug.Log("bridge: nodeWrapper.Open")
	opener, ok := w.node.(NodeOpener)
	if !ok {
		return nil, syscall.ENOSYS
	}

	ourReq := &OpenRequest{Flags: OpenFlags(req.Flags)}
	var ourResp OpenResponse

	handle, err := opener.Open(ctx, ourReq, &ourResp)
	if err != nil {
		return nil, err
	}

	resp.Flags = fuse.OpenResponseFlags(ourResp.Flags)
	return &handleWrapper{handle: handle}, nil
}

func (w *nodeWrapper) Getxattr(ctx context.Context, req *fuse.GetxattrRequest, resp *fuse.GetxattrResponse) error {
	debug.Log("bridge: nodeWrapper.Getxattr")
	getxattrer, ok := w.node.(NodeGetxattrer)
	if !ok {
		return syscall.ENOSYS
	}

	ourReq := &GetxattrRequest{Name: req.Name, Size: req.Size}
	var ourResp GetxattrResponse

	err := getxattrer.Getxattr(ctx, ourReq, &ourResp)
	if err != nil {
		return err
	}
	resp.Xattr = ourResp.Xattr
	return nil
}

func (w *nodeWrapper) Listxattr(ctx context.Context, req *fuse.ListxattrRequest, resp *fuse.ListxattrResponse) error {
	debug.Log("bridge: nodeWrapper.Listxattr")
	listxattrer, ok := w.node.(NodeListxattrer)
	if !ok {
		return syscall.ENOSYS
	}

	ourReq := &ListxattrRequest{Size: req.Size}
	var ourResp ListxattrResponse

	err := listxattrer.Listxattr(ctx, ourReq, &ourResp)
	if err != nil {
		return err
	}
	resp.Xattr = ourResp.Xattr
	return nil
}

func (w *handleWrapper) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	debug.Log("bridge: handleWrapper.Read")
	reader, ok := w.handle.(HandleReader)
	if !ok {
		return syscall.ENOSYS
	}

	ourReq := &ReadRequest{Offset: req.Offset, Size: req.Size}
	ourResp := ReadResponse{Data: resp.Data}

	err := reader.Read(ctx, ourReq, &ourResp)
	resp.Data = ourResp.Data
	return err
}

func (w *nodeWrapper) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	debug.Log("bridge: handleWrapper.ReadDirAll")
	reader, ok := w.node.(HandleReadDirAller)
	if !ok {
		return nil, syscall.ENOSYS
	}

	ourDirents, err := reader.ReadDirAll(ctx)
	if err != nil {
		return nil, err
	}

	// convert dirents
	fuseDirents := make([]fuse.Dirent, len(ourDirents))
	for i, d := range ourDirents {
		fuseDirents[i] = fuse.Dirent{
			Inode: d.Inode,
			Type:  fuse.DirentType(d.Type),
			Name:  d.Name,
		}
	}

	return fuseDirents, nil
}
