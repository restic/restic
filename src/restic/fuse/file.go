// +build !openbsd
// +build !windows

package fuse

import (
	"errors"
	"sync"

	"restic"
	"restic/backend"
	"restic/debug"
	"restic/pack"

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
	LookupBlobSize(backend.ID, pack.BlobType) (uint, error)
	LoadBlob(backend.ID, pack.BlobType, []byte) ([]byte, error)
}

type file struct {
	repo        BlobLoader
	node        *restic.Node
	ownerIsRoot bool

	sizes []uint
	blobs [][]byte
}

const defaultBlobSize = 128 * 1024

var blobPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, defaultBlobSize)
	},
}

func newFile(repo BlobLoader, node *restic.Node, ownerIsRoot bool) (*file, error) {
	debug.Log("newFile", "create new file for %v with %d blobs", node.Name, len(node.Content))
	var bytes uint64
	sizes := make([]uint, len(node.Content))
	for i, id := range node.Content {
		size, err := repo.LookupBlobSize(id, pack.Data)
		if err != nil {
			return nil, err
		}

		sizes[i] = size
		bytes += uint64(size)
	}

	if bytes != node.Size {
		debug.Log("newFile", "sizes do not match: node.Size %v != size %v, using real size", node.Size, bytes)
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
	debug.Log("file.Attr", "Attr(%v)", f.node.Name)
	a.Inode = f.node.Inode
	a.Mode = f.node.Mode
	a.Size = f.node.Size
	a.Blocks = (f.node.Size / blockSize) + 1
	a.BlockSize = blockSize

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
	debug.Log("file.getBlobAt", "getBlobAt(%v, %v)", f.node.Name, i)
	if f.blobs[i] != nil {
		return f.blobs[i], nil
	}

	buf := blobPool.Get().([]byte)
	buf = buf[:cap(buf)]

	if uint(len(buf)) < f.sizes[i] {
		if len(buf) > defaultBlobSize {
			blobPool.Put(buf)
		}
		buf = make([]byte, f.sizes[i])
	}

	blob, err = f.repo.LoadBlob(f.node.Content[i], pack.Data, buf)
	if err != nil {
		debug.Log("file.getBlobAt", "LoadBlob(%v, %v) failed: %v", f.node.Name, f.node.Content[i], err)
		return nil, err
	}
	f.blobs[i] = blob

	return blob, nil
}

func (f *file) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	debug.Log("file.Read", "Read(%v, %v, %v), file size %v", f.node.Name, req.Size, req.Offset, f.node.Size)
	offset := req.Offset

	if uint64(offset) > f.node.Size {
		debug.Log("file.Read", "Read(%v): offset is greater than file size: %v > %v",
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
		if f.blobs[i] != nil {
			blobPool.Put(f.blobs[i])
			f.blobs[i] = nil
		}
	}
	return nil
}
