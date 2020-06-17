// +build darwin freebsd linux

package fuse

import (
	"sort"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/debug"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"golang.org/x/net/context"
)

// The default block size to report in stat
const blockSize = 512

// Statically ensure that *file implements the given interface
var _ = fs.HandleReader(&file{})

type file struct {
	root  *Root
	node  *restic.Node
	inode uint64

	// cumsize[i] holds the cumulative size of blobs[:i].
	cumsize []uint64
}

func newFile(ctx context.Context, root *Root, inode uint64, node *restic.Node) (fusefile *file, err error) {
	debug.Log("create new file for %v with %d blobs", node.Name, len(node.Content))
	var bytes uint64
	cumsize := make([]uint64, 1+len(node.Content))
	for i, id := range node.Content {
		size, ok := root.blobSizeCache.Lookup(id)
		if !ok {
			var found bool
			size, found = root.repo.LookupBlobSize(id, restic.DataBlob)
			if !found {
				return nil, errors.Errorf("id %v not found in repository", id)
			}
		}

		bytes += uint64(size)
		cumsize[i+1] = bytes
	}

	if bytes != node.Size {
		debug.Log("sizes do not match: node.Size %v != size %v, using real size", node.Size, bytes)
		node.Size = bytes
	}

	return &file{
		inode: inode,
		root:  root,
		node:  node,

		cumsize: cumsize,
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

func (f *file) getBlobAt(ctx context.Context, i int) (blob []byte, err error) {
	debug.Log("getBlobAt(%v, %v)", f.node.Name, i)

	blob, ok := f.root.blobCache.get(f.node.Content[i])
	if ok {
		return blob, nil
	}

	blob, err = f.root.repo.LoadBlob(ctx, restic.DataBlob, f.node.Content[i], nil)
	if err != nil {
		debug.Log("LoadBlob(%v, %v) failed: %v", f.node.Name, f.node.Content[i], err)
		return nil, err
	}

	f.root.blobCache.add(f.node.Content[i], blob)

	return blob, nil
}

func (f *file) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	debug.Log("Read(%v, %v, %v), file size %v", f.node.Name, req.Size, req.Offset, f.node.Size)
	offset := uint64(req.Offset)

	if offset > f.node.Size {
		debug.Log("Read(%v): offset is greater than file size: %v > %v",
			f.node.Name, req.Offset, f.node.Size)

		// return no data
		resp.Data = resp.Data[:0]
		return nil
	}

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
