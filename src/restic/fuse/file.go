// +build !openbsd
// +build !windows

package fuse

import (
	"restic/errors"

	"restic"
	"restic/debug"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"golang.org/x/net/context"
)

// The default block size to report in stat
const blockSize = 512

// Statically ensure that *file implements the given interface
var _ = fs.HandleReader(&file{})
var _ = fs.HandleReleaser(&file{})

// BlobLoader is an abstracted repository with a reduced set of methods used
// for fuse operations.
type BlobLoader interface {
	LookupBlobSize(restic.ID, restic.BlobType) (uint, error)
	LoadBlob(restic.BlobType, restic.ID, []byte) (int, error)
}

type file struct {
	repo        BlobLoader
	node        *restic.Node
	ownerIsRoot bool

	sizes []int
	blobs [][]byte
}

const defaultBlobSize = 128 * 1024

func newFile(repo BlobLoader, node *restic.Node, ownerIsRoot bool) (*file, error) {
	debug.Log("create new file for %v with %d blobs", node.Name, len(node.Content))
	var bytes uint64
	sizes := make([]int, len(node.Content))
	for i, id := range node.Content {
		size, err := repo.LookupBlobSize(id, restic.DataBlob)
		if err != nil {
			return nil, err
		}

		sizes[i] = int(size)
		bytes += uint64(size)
	}

	if bytes != node.Size {
		debug.Log("sizes do not match: node.Size %v != size %v, using real size", node.Size, bytes)
		node.Size = bytes
	}

	return &file{
		repo:        repo,
		node:        node,
		sizes:       sizes,
		blobs:       make([][]byte, len(node.Content)),
		ownerIsRoot: ownerIsRoot,
	}, nil
}

func (f *file) Attr(ctx context.Context, a *fuse.Attr) error {
	debug.Log("Attr(%v)", f.node.Name)
	a.Inode = f.node.Inode
	a.Mode = f.node.Mode
	a.Size = f.node.Size
	a.Blocks = (f.node.Size / blockSize) + 1
	a.BlockSize = blockSize
	a.Nlink = uint32(f.node.Links)

	if !f.ownerIsRoot {
		a.Uid = f.node.UID
		a.Gid = f.node.GID
	}
	a.Atime = f.node.AccessTime
	a.Ctime = f.node.ChangeTime
	a.Mtime = f.node.ModTime

	return nil

}

func (f *file) getBlobAt(i int) (blob []byte, err error) {
	debug.Log("getBlobAt(%v, %v)", f.node.Name, i)
	if f.blobs[i] != nil {
		return f.blobs[i], nil
	}

	// release earlier blobs
	for j := 0; j < i; j++ {
		f.blobs[j] = nil
	}

	buf := restic.NewBlobBuffer(f.sizes[i])
	n, err := f.repo.LoadBlob(restic.DataBlob, f.node.Content[i], buf)
	if err != nil {
		debug.Log("LoadBlob(%v, %v) failed: %v", f.node.Name, f.node.Content[i], err)
		return nil, err
	}
	f.blobs[i] = buf[:n]

	return buf[:n], nil
}

func (f *file) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	debug.Log("Read(%v, %v, %v), file size %v", f.node.Name, req.Size, req.Offset, f.node.Size)
	offset := req.Offset

	if uint64(offset) > f.node.Size {
		debug.Log("Read(%v): offset is greater than file size: %v > %v",
			f.node.Name, req.Offset, f.node.Size)
		return errors.New("offset greater than files size")
	}

	// handle special case: file is empty
	if f.node.Size == 0 {
		resp.Data = resp.Data[:0]
		return nil
	}

	// Skip blobs before the offset
	startContent := 0
	for offset > int64(f.sizes[startContent]) {
		offset -= int64(f.sizes[startContent])
		startContent++
	}

	dst := resp.Data[0:req.Size]
	readBytes := 0
	remainingBytes := req.Size
	for i := startContent; remainingBytes > 0 && i < len(f.sizes); i++ {
		blob, err := f.getBlobAt(i)
		if err != nil {
			return err
		}

		if offset > 0 {
			blob = blob[offset:len(blob)]
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

func (f *file) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	for i := range f.blobs {
		f.blobs[i] = nil
	}
	return nil
}

func (f *file) Listxattr(ctx context.Context, req *fuse.ListxattrRequest, resp *fuse.ListxattrResponse) error {
	debug.Log("Listxattr(%v, %v)", f.node.Name, req.Size)
	for k := range f.node.ExtendedAttributes {
		resp.Append(k)
	}
	return nil
}

func (f *file) Getxattr(ctx context.Context, req *fuse.GetxattrRequest, resp *fuse.GetxattrResponse) error {
	debug.Log("Getxattr(%v, %v, %v)", f.node.Name, req.Name, req.Size)
	if val, ok := f.node.ExtendedAttributes[req.Name]; ok {
		resp.Xattr = val
		return nil
	}
	return fuse.ErrNoXattr
}
