package fs

import (
	"bazil.org/fuse"
	"context"
	"encoding/binary"
	"hash/fnv"
)

// A Node is the interface required of a file or directory.
// See the documentation for type FS for general information
// pertaining to all methods.
//
// A Node must be usable as a map key, that is, it cannot be a
// function, map or slice.
//
// Other FUSE requests can be handled by implementing methods from the
// Node* interfaces, for example NodeOpener.
//
// Methods returning Node should take care to return the same Node
// when the result is logically the same instance. Without this, each
// Node will get a new NodeID, causing spurious cache invalidations,
// extra lookups and aliasing anomalies. This may not matter for a
// simple, read-only filesystem.
type Node interface {
	// Attr fills attr with the standard metadata for the node.
	//
	// Fields with reasonable defaults are prepopulated. For example,
	// all times are set to a fixed moment when the program started.
	//
	// If Inode is left as 0, a dynamic inode number is chosen.
	//
	// The result may be cached for the duration set in Valid.
	Attr(ctx context.Context, attr *fuse.Attr) error
}

type HandleReadDirAller interface {
	ReadDirAll(ctx context.Context) ([]fuse.Dirent, error)
}
type HandleReadAller interface {
	ReadAll(ctx context.Context) ([]byte, error)
}

type NodeStringLookuper interface {
	// Lookup looks up a specific entry in the receiver,
	// which must be a directory.  Lookup should return a Node
	// corresponding to the entry.  If the name does not exist in
	// the directory, Lookup should return ENOENT.
	//
	// Lookup need not to handle the names "." and "..".
	Lookup(ctx context.Context, name string) (Node, error)
}

type HandleReader interface {
	// Read requests to read data from the handle.
	//
	// There is a page cache in the kernel that normally submits only
	// page-aligned reads spanning one or more pages. However, you
	// should not rely on this. To see individual requests as
	// submitted by the file system clients, set OpenDirectIO.
	//
	// Note that reads beyond the size of the file as reported by Attr
	// are not even attempted (except in OpenDirectIO mode).

	Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error
}
type HandleReleaser interface {
	Release(ctx context.Context, req *fuse.ReleaseRequest) error
}

// This optional request will be called only for symbolic link nodes.
type NodeReadlinker interface {
	// Readlink reads a symbolic link.
	Readlink(ctx context.Context, req *fuse.ReadlinkRequest) (string, error)
}

// GenerateDynamicInode returns a dynamic inode.
//
// The parent inode and current entry name are used as the criteria
// for choosing a pseudorandom inode. This makes it likely the same
// entry will get the same inode on multiple runs.
func GenerateDynamicInode(parent uint64, name string) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], parent)
	_, _ = h.Write(buf[:])
	_, _ = h.Write([]byte(name))
	var inode uint64
	for {
		inode = h.Sum64()
		if inode != 0 {
			break
		}
		// there's a tiny probability that result is zero; change the
		// input a little and try again
		_, _ = h.Write([]byte{'x'})
	}
	return inode
}
