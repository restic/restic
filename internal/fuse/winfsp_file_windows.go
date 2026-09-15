package fuse

import (
	"io"
	"os"
	"sort"
	"syscall"

	"github.com/winfsp/go-winfsp/gofs"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

// winFile is an open file. Reads are served from the blobs of the node.
type winFile struct {
	readOnlyFile
	fs   *WinFS
	node *data.Node
	// cumsize[i] holds the cumulative size of node.Content[:i].
	cumsize []uint64
	offset  int64
}

var _ gofs.File = (*winFile)(nil)

func (fs *WinFS) openFile(node *data.Node) (*winFile, error) {
	debug.Log("open file %v with %d blobs", node.Name, len(node.Content))

	var size uint64
	cumsize := make([]uint64, 1+len(node.Content))
	for i, id := range node.Content {
		blobSize, found := fs.repo.LookupBlobSize(restic.BlobHandle{Type: restic.DataBlob, ID: id})
		if !found {
			return nil, errors.Errorf("id %v not found in repository", id)
		}

		size += uint64(blobSize)
		cumsize[i+1] = size
	}

	if size != node.Size {
		debug.Log("sizes do not match: node.Size %v != size %v, using real size", node.Size, size)
		// Make a copy of the node with correct size
		nodenew := *node
		nodenew.Size = size
		node = &nodenew
	}

	return &winFile{fs: fs, node: node, cumsize: cumsize}, nil
}

func (f *winFile) getBlobAt(i int) ([]byte, error) {
	blob, err := f.fs.blobCache.GetOrCompute(f.node.Content[i], func() ([]byte, error) {
		return f.fs.repo.LoadBlob(f.fs.ctx, restic.BlobHandle{Type: restic.DataBlob, ID: f.node.Content[i]}, nil)
	})
	if err != nil {
		debug.Log("LoadBlob(%v, %v) failed: %v", f.node.Name, f.node.Content[i], err)
		return nil, err
	}

	return blob, nil
}

func (f *winFile) ReadAt(p []byte, off int64) (int, error) {
	debug.Log("ReadAt(%v, %v, %v), file size %v", f.node.Name, len(p), off, f.node.Size)
	if off < 0 {
		return 0, syscall.EINVAL
	}
	offset := uint64(off)
	if offset >= f.node.Size {
		return 0, io.EOF
	}

	// Skip blobs before the offset
	startContent := -1 + sort.Search(len(f.cumsize), func(i int) bool {
		return f.cumsize[i] > offset
	})
	offset -= f.cumsize[startContent]

	n := 0
	for i := startContent; n < len(p) && i < len(f.cumsize)-1; i++ {
		blob, err := f.getBlobAt(i)
		if err != nil {
			return n, err
		}

		if offset > 0 {
			blob = blob[offset:]
			offset = 0
		}

		n += copy(p[n:], blob)
	}

	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (f *winFile) Read(p []byte) (int, error) {
	n, err := f.ReadAt(p, f.offset)
	f.offset += int64(n)
	return n, err
}

func (f *winFile) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = f.offset + offset
	case io.SeekEnd:
		abs = int64(f.node.Size) + offset
	default:
		return 0, syscall.EINVAL
	}
	if abs < 0 {
		return 0, syscall.EINVAL
	}
	f.offset = abs
	return abs, nil
}

func (f *winFile) Stat() (os.FileInfo, error)         { return winFileInfo{f.node}, nil }
func (f *winFile) Readdir(int) ([]os.FileInfo, error) { return nil, syscall.ENOTDIR }
