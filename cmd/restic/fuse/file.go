package fuse

import (
	"github.com/restic/restic"
	"github.com/restic/restic/pack"
	"github.com/restic/restic/repository"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"golang.org/x/net/context"
)

// Statically ensure that *file implements the given interface
var _ = fs.HandleReader(&file{})

type file struct {
	repo *repository.Repository
	node *restic.Node

	sizes []uint32
	blobs [][]byte
}

func newFile(repo *repository.Repository, node *restic.Node) (*file, error) {
	sizes := make([]uint32, len(node.Content))
	for i, blobId := range node.Content {
		length, err := repo.Index().LookupSize(blobId)
		if err != nil {
			return nil, err
		}
		sizes[i] = uint32(length)
	}

	return &file{
		repo:  repo,
		node:  node,
		sizes: sizes,
		blobs: make([][]byte, len(node.Content)),
	}, nil
}

func (f *file) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = f.node.Inode
	a.Mode = f.node.Mode
	a.Size = f.node.Size
	return nil
}

func (f *file) getBlobAt(i int) (blob []byte, err error) {
	if f.blobs[i] != nil {
		blob = f.blobs[i]
	} else {
		blob, err = f.repo.LoadBlob(pack.Data, f.node.Content[i])
		if err != nil {
			return nil, err
		}
		f.blobs[i] = blob
	}

	return blob, nil
}

func (f *file) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	off := req.Offset

	// Skip blobs before the offset
	startContent := 0
	for off > int64(f.sizes[startContent]) {
		off -= int64(f.sizes[startContent])
		startContent++
	}

	content := make([]byte, req.Size)
	allContent := content
	for i := startContent; i < len(f.sizes); i++ {
		blob, err := f.getBlobAt(i)
		if err != nil {
			return err
		}

		blob = blob[off:]
		off = 0

		var copied int
		if len(blob) > len(content) {
			copied = copy(content[0:], blob[:len(content)])
		} else {
			copied = copy(content[0:], blob)
		}
		content = content[copied:]
		if len(content) == 0 {
			break
		}
	}
	resp.Data = allContent
	return nil
}
