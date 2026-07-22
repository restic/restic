package object_test

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"testing"

	"github.com/restic/restic/internal/object"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

type loader func(context.Context, restic.BlobHandle, []byte) ([]byte, error)

func (l loader) LoadBlob(ctx context.Context, h restic.BlobHandle, buf []byte) ([]byte, error) {
	return l(ctx, h, buf)
}

func TestReader(t *testing.T) {
	seed := int64(42)
	src := rand.New(rand.NewSource(seed))
	data := make(map[restic.ID][]byte)

	var full []byte
	var ids restic.IDs

	for i := 0; i < 3; i++ {
		buf := make([]byte, 100)
		_, err := src.Read(buf)
		rtest.OK(t, err)
		id := restic.Hash(buf)
		data[id] = buf

		full = append(full, buf...)
		ids = append(ids, id)
	}

	l := loader(func(ctx context.Context, h restic.BlobHandle, b []byte) ([]byte, error) {
		if h.Type != restic.DataBlob {
			return nil, errors.New("wrong type")
		}
		val, ok := data[h.ID]
		if !ok {
			return nil, errors.New("unknown id")
		}
		return val, nil
	})

	rd := object.NewReader(context.TODO(), l, restic.DataBlob, ids)
	read, err := io.ReadAll(rd)
	rtest.OK(t, err)
	rtest.Equals(t, full, read)
}

func TestReaderErrors(t *testing.T) {
	loadErr := errors.New("load failed")

	t.Run("empty", func(t *testing.T) {
		rd := object.NewReader(context.TODO(), loader(func(context.Context, restic.BlobHandle, []byte) ([]byte, error) {
			return nil, errors.New("unexpected load")
		}), restic.DataBlob, nil)

		n, err := rd.Read(make([]byte, 10))
		rtest.Equals(t, 0, n)
		rtest.Equals(t, io.EOF, err)
	})

	t.Run("loadError", func(t *testing.T) {
		id1 := restic.Hash([]byte("first"))
		id2 := restic.Hash([]byte("second"))
		ids := restic.IDs{id1, id2}

		rd := object.NewReader(context.TODO(), loader(func(_ context.Context, h restic.BlobHandle, _ []byte) ([]byte, error) {
			if h.ID == id1 {
				return []byte("first"), nil
			}
			return nil, loadErr
		}), restic.DataBlob, ids)

		buf := make([]byte, 64)
		n, err := rd.Read(buf)
		rtest.OK(t, err)
		rtest.Equals(t, "first", string(buf[:n]))

		n, err = rd.Read(buf)
		rtest.Equals(t, 0, n)
		rtest.Equals(t, loadErr, err)
	})

	t.Run("sticky", func(t *testing.T) {
		id1 := restic.Hash([]byte("first"))
		ids := restic.IDs{id1}

		rd := object.NewReader(context.TODO(), loader(func(_ context.Context, h restic.BlobHandle, _ []byte) ([]byte, error) {
			return nil, loadErr
		}), restic.DataBlob, ids)

		buf := make([]byte, 64)
		_, err := rd.Read(buf)
		rtest.Equals(t, loadErr, err)

		n, err := rd.Read(buf)
		rtest.Equals(t, 0, n)
		rtest.Equals(t, "reader unusable after error", err.Error())
	})
}

func TestReaderPartialRead(t *testing.T) {
	blob1 := []byte("abcdefgh")
	blob2 := []byte("ijklmnop")
	id1 := restic.Hash(blob1)
	id2 := restic.Hash(blob2)
	ids := restic.IDs{id1, id2}
	want := append(append([]byte{}, blob1...), blob2...)

	rd := object.NewReader(context.TODO(), loader(func(_ context.Context, h restic.BlobHandle, _ []byte) ([]byte, error) {
		switch h.ID {
		case id1:
			return blob1, nil
		case id2:
			return blob2, nil
		default:
			return nil, errors.New("unknown id")
		}
	}), restic.DataBlob, ids)

	var got []byte
	buf := make([]byte, 7)
	for {
		n, err := rd.Read(buf)
		if n > 0 {
			got = append(got, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		rtest.OK(t, err)
	}
	rtest.Equals(t, want, got)
}
