package object

import (
	"context"
	"errors"
	"io"

	"github.com/restic/restic/internal/restic"
)

type reader struct {
	ctx    context.Context
	loader restic.BlobLoader
	tpe    restic.BlobType

	content restic.IDs
	data    []byte
	buf     []byte // points to the same buffer as data, but is not advanced to allow reuse of the buffer
	err     error
}

// NewReader returns a sequential io.Reader that loads blobs one at a time.
func NewReader(ctx context.Context, loader restic.BlobLoader, tpe restic.BlobType, content restic.IDs) io.Reader {
	return &reader{
		ctx:     ctx,
		loader:  loader,
		tpe:     tpe,
		content: content,
	}
}

func (r *reader) Read(p []byte) (n int, err error) {
	if r.err != nil {
		return 0, r.err
	}

	// return already buffered data
	if r.data != nil {
		n = copy(p, r.data)
		r.data = r.data[n:]
		if len(r.data) == 0 {
			r.data = nil
		}
		return n, nil
	}

	// no more data to read
	if len(r.content) == 0 {
		return 0, io.EOF
	}

	// read next blob
	id := r.content[0]
	r.content = r.content[1:]

	blob, err := r.loader.LoadBlob(r.ctx, restic.BlobHandle{Type: r.tpe, ID: id}, r.buf)
	if err != nil {
		r.err = errors.New("reader unusable after error")
		return 0, err
	}

	r.data = blob
	r.buf = blob
	return r.Read(p)
}
