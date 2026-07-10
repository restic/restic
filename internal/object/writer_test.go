package object_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/restic/restic/internal/object"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

type saveBlobAsyncStub struct {
	buffer bytes.Buffer
	ids    restic.IDs
	pool   *restic.BlobBufferPool
}

func (s *saveBlobAsyncStub) BlobBufferPool() *restic.BlobBufferPool {
	return s.pool
}

func (s *saveBlobAsyncStub) SaveBlobAsync(ctx context.Context, t restic.BlobType, buf *restic.BlobBuffer, id restic.ID, storeDuplicate bool, cb func(newID restic.ID, known bool, size int, err error)) {
	s.buffer.Write(buf.Data)
	newID := restic.Hash(buf.Data)
	s.ids = append(s.ids, newID)
	cb(newID, false, len(buf.Data), nil)
}

type chunkOnByteStub struct {
	splitByte byte
}

func (c *chunkOnByteStub) Reset() {}

func (c *chunkOnByteStub) NextSplitPoint(buf []byte) int {
	for i, b := range buf {
		if b == c.splitByte {
			return i + 1
		}
	}
	return -1
}

func TestWriter(t *testing.T) {
	data := []byte{1, 2, 42, 3, 4, 42, 5, 6}

	chnkr := &chunkOnByteStub{splitByte: 42}
	saver := &saveBlobAsyncStub{pool: restic.NewBlobBufferPool(len(data))}
	w := object.NewWriter(context.TODO(), restic.DataBlob, chnkr, saver)

	_, err := w.Write(data)
	rtest.OK(t, err)

	ids, err := w.Flush()
	rtest.OK(t, err)

	rtest.Equals(t, 3, len(ids))
	rtest.Equals(t, saver.ids, ids)
	rtest.Assert(t, bytes.Equal(saver.buffer.Bytes(), data), "data differs")
}
