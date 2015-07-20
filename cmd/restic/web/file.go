package web

import (
	"os"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/pack"
	"github.com/restic/restic/repository"
)

type File struct {
	repo  *repository.Repository
	blobs []*lazyBlob
	off   int64
}

func NewFile(repo *repository.Repository, node *restic.Node) (*File, error) {
	if len(node.Content) == 0 {
		return nil, nil
	}

	blobs := make([]*lazyBlob, len(node.Content))
	for i, blobId := range node.Content {
		blob, err := newBlob(repo, blobId)
		if err != nil {
			return nil, err
		}
		blobs[i] = blob
	}

	return &File{
		repo:  repo,
		blobs: blobs,
	}, nil
}

type lazyBlob struct {
	repo *repository.Repository
	id   backend.ID
	size int64

	// lazily loaded
	content []byte
}

func newBlob(repo *repository.Repository, id backend.ID) (*lazyBlob, error) {
	size, err := repo.Index().LookupSize(id)
	if err != nil {
		return nil, err
	}
	return &lazyBlob{
		repo: repo,
		id:   id,
		size: int64(size),
	}, nil
}

func (lb lazyBlob) ReadAt(p []byte, off int64) (n int, err error) {
	if len(lb.content) == 0 {
		lb.content, err = lb.repo.LoadBlob(pack.Data, lb.id)
		if err != nil {
			return 0, err
		}
	}

	return copy(p, lb.content[off:]), nil
}

func (f *File) Read(p []byte) (n int, err error) {
	off := f.off
	contentIdx := 0
	for off > f.blobs[contentIdx].size {
		off -= f.blobs[contentIdx].size
		contentIdx++
	}

	nr := 0

	for len(p) > 0 && contentIdx < len(f.blobs) {
		n1, err1 := f.blobs[contentIdx].ReadAt(p, off)
		if err1 != nil {
			return nr, err1
		}
		nr += n1
		p = p[n1:]
		off = 0
		contentIdx++
		f.off += int64(n1)
	}

	return nr, nil
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case os.SEEK_SET:
		f.off = offset
	case os.SEEK_CUR:
		f.off += offset
	case os.SEEK_END:
		var totalSize int64
		for _, blob := range f.blobs {
			totalSize += blob.size
		}
		f.off = totalSize - offset
	}
	return f.off, nil
}
