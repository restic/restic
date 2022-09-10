//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fuse

import (
	"context"
	"sort"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// The default block size to report in stat
const blockSize = 512

// Statically ensure that *file and *openFile implement the given interfaces
var _ = fs.HandleReader(&openFile{})
var _ = fs.NodeListxattrer(&file{})
var _ = fs.NodeGetxattrer(&file{})
var _ = fs.NodeOpener(&file{})

type file struct {
	root  *Root
	node  *restic.Node
	inode uint64
}

type openFile struct {
	file
	// cumsize[i] holds the cumulative size of blobs[:i].
	cumsize []uint64
}

func newFile(root *Root, inode uint64, node *restic.Node) (fusefile *file, err error) {
	debug.Log("create new file for %v with %d blobs", node.Name, len(node.Content))
	return &file{
		inode: inode,
		root:  root,
		node:  node,
	}, nil
}

func (f *file) Attr(ctx context.Context, a *fuse.Attr) error {
	debug.Log("Attr(%v)", f.node.Name)
	a.Inode = f.inode
	a.Mode = f.node.Mode
	a.Size = f.node.Size
	a.Blocks = (f.node.Size / blockSize) + 1
	a.BlockSize = blockSize
	a.Nlink = uint32(f.node.Links)

	if !f.root.cfg.OwnerIsRoot {
		a.Uid = f.node.UID
		a.Gid = f.node.GID
	}
	a.Atime = f.node.AccessTime
	a.Ctime = f.node.ChangeTime
	a.Mtime = f.node.ModTime

	return nil

}

func (f *file) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	debug.Log("open file %v with %d blobs", f.node.Name, len(f.node.Content))

	var bytes uint64
	cumsize := make([]uint64, 1+len(f.node.Content))
	for i, id := range f.node.Content {
		size, found := f.root.repo.LookupBlobSize(id, restic.DataBlob)
		if !found {
			return nil, errors.Errorf("id %v not found in repository", id)
		}

		bytes += uint64(size)
		cumsize[i+1] = bytes
	}

	var of = openFile{file: *f}

	if bytes != f.node.Size {
		debug.Log("sizes do not match: node.Size %v != size %v, using real size", f.node.Size, bytes)
		// Make a copy of the node with correct size
		nodenew := *f.node
		nodenew.Size = bytes
		of.file.node = &nodenew
	}
	of.cumsize = cumsize

	return &of, nil
}

func (f *openFile) getBlobAt(ctx context.Context, i int) (blob []byte, err error) {

	blob, ok := f.root.blobCache.Get(f.node.Content[i])
	if ok {
		return blob, nil
	}

	blob, err = f.root.repo.LoadBlob(ctx, restic.DataBlob, f.node.Content[i], nil)
	if err != nil {
		debug.Log("LoadBlob(%v, %v) failed: %v", f.node.Name, f.node.Content[i], err)
		return nil, unwrapCtxCanceled(err)
	}

	f.root.blobCache.Add(f.node.Content[i], blob)

	return blob, nil
}

func (f *openFile) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	debug.Log("Read(%v, %v, %v), file size %v", f.node.Name, req.Size, req.Offset, f.node.Size)
	offset := uint64(req.Offset)

	// as stated in https://godoc.org/bazil.org/fuse/fs#HandleReader there
	// is no need to check if offset > size

	// handle special case: file is empty
	if f.node.Size == 0 {
		resp.Data = resp.Data[:0]
		return nil
	}

	// Skip blobs before the offset
	startContent := -1 + sort.Search(len(f.cumsize), func(i int) bool {
		return f.cumsize[i] > offset
	})
	offset -= f.cumsize[startContent]

	dst := resp.Data[0:req.Size]
	readBytes := 0
	remainingBytes := req.Size

	// The documentation of bazil/fuse actually says that synchronization is
	// required (see https://godoc.org/bazil.org/fuse#hdr-Service_Methods):
	//
	// Multiple goroutines may call service methods simultaneously;
	// the methods being called are responsible for appropriate synchronization.
	//
	// However, no lock needed here as getBlobAt can be called conurrently
	// (blobCache has it's own locking)
	for i := startContent; remainingBytes > 0 && i < len(f.cumsize)-1; i++ {
		blob, err := f.getBlobAt(ctx, i)
		if err != nil {
			return err
		}

		if offset > 0 {
			blob = blob[offset:]
			offset = 0
		}

		copied := copy(dst, blob)
		remainingBytes -= copied
		readBytes += copied

		dst = dst[copied:]
	}
	resp.Data = resp.Data[:readBytes]

	return nil
}

func (f *file) Listxattr(ctx context.Context, req *fuse.ListxattrRequest, resp *fuse.ListxattrResponse) error {
	debug.Log("Listxattr(%v, %v)", f.node.Name, req.Size)
	for _, attr := range f.node.ExtendedAttributes {
		resp.Append(attr.Name)
	}
	return nil
}

func (f *file) Getxattr(ctx context.Context, req *fuse.GetxattrRequest, resp *fuse.GetxattrResponse) error {
	debug.Log("Getxattr(%v, %v, %v)", f.node.Name, req.Name, req.Size)
	attrval := f.node.GetExtendedAttribute(req.Name)
	if attrval != nil {
		resp.Xattr = attrval
		return nil
	}
	return fuse.ErrNoXattr
}
