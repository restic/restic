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
