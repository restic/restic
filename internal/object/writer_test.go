package object_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/restic/restic/internal/object"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

type saveBlobAsyncStub struct {
	buffer bytes.Buffer
	ids    restic.IDs
	pool   *restic.BlobBufferPool
	known  bool
	err    error
}

func (s *saveBlobAsyncStub) BlobBufferPool() *restic.BlobBufferPool {
	return s.pool
}

func (s *saveBlobAsyncStub) SaveBlobAsync(ctx context.Context, t restic.BlobType, buf *restic.BlobBuffer, id restic.ID, storeDuplicate bool, cb func(newID restic.ID, known bool, size int, err error)) {
	if s.err != nil {
		cb(restic.ID{}, false, 0, s.err)
		return
	}
	s.buffer.Write(buf.Data)
	newID := restic.Hash(buf.Data)
	s.ids = append(s.ids, newID)
	cb(newID, s.known, len(buf.Data), nil)
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

func TestWriterStats(t *testing.T) {
	data := []byte{1, 2, 42, 3, 4, 42, 5, 6}

	for _, tt := range []struct {
		name           string
		known          bool
		wantBlobs      int
		wantSize       uint64
		wantSizeInRepo uint64
	}{
		{
			name:           "new-blobs",
			wantBlobs:      3,
			wantSize:       uint64(len(data)),
			wantSizeInRepo: uint64(len(data)),
		},
		{
			name:  "known",
			known: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			saver := &saveBlobAsyncStub{pool: restic.NewBlobBufferPool(len(data)), known: tt.known}
			w := object.NewWriter(context.TODO(), restic.DataBlob, &chunkOnByteStub{splitByte: 42}, saver)

			_, err := w.Write(data)
			rtest.OK(t, err)
			_, err = w.Flush()
			rtest.OK(t, err)

			blobs, size, sizeInRepo := w.Stats()
			rtest.Equals(t, tt.wantBlobs, blobs)
			rtest.Equals(t, tt.wantSize, size)
			rtest.Equals(t, tt.wantSizeInRepo, sizeInRepo)
		})
	}
}

func TestWriterWriteAfterFlush(t *testing.T) {
	saver := &saveBlobAsyncStub{pool: restic.NewBlobBufferPool(8)}
	w := object.NewWriter(context.TODO(), restic.DataBlob, &chunkOnByteStub{splitByte: 42}, saver)

	_, err := w.Flush()
	rtest.OK(t, err)
	n, err := w.Write([]byte{3})
	rtest.Equals(t, 0, n)
	rtest.Equals(t, "writer already flushed", err.Error())
}

func TestWriterSaveError(t *testing.T) {
	saveErr := errors.New("save failed")
	saver := &saveBlobAsyncStub{pool: restic.NewBlobBufferPool(8), err: saveErr}
	w := object.NewWriter(context.TODO(), restic.DataBlob, &chunkOnByteStub{splitByte: 42}, saver)

	_, err := w.Write([]byte{1, 42, 2})
	rtest.OK(t, err)

	_, gotErr := w.Flush()
	rtest.Equals(t, saveErr, gotErr)
}
