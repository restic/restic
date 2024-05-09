package repository_test

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/backend/mock"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

const KiB = 1 << 10
const MiB = 1 << 20

func TestLoadAll(t *testing.T) {
	b := mem.New()
	repo, err := repository.New(b, repository.Options{})
	rtest.OK(t, err)

	for i := 0; i < 5; i++ {
		data := rtest.Random(23+i, rand.Intn(MiB)+500*KiB)

		id := restic.Hash(data)
		h := backend.Handle{Name: id.String(), Type: backend.PackFile}
		err := b.Save(context.TODO(), h, backend.NewByteReader(data, b.Hasher()))
		rtest.OK(t, err)

		buf, err := repo.LoadRaw(context.TODO(), backend.PackFile, id)
		rtest.OK(t, err)

		if len(buf) != len(data) {
			t.Errorf("length of returned buffer does not match, want %d, got %d", len(data), len(buf))
			continue
		}

		if !bytes.Equal(buf, data) {
			t.Errorf("wrong data returned")
			continue
		}
	}
}

type quickRetryBackend struct {
	backend.Backend
}

func (be *quickRetryBackend) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	err := be.Backend.Load(ctx, h, length, offset, fn)
	if err != nil {
		// retry
		err = be.Backend.Load(ctx, h, length, offset, fn)
	}
	return err
}

func TestLoadAllBroken(t *testing.T) {
	b := mock.NewBackend()
	repo, err := repository.New(b, repository.Options{})
	rtest.OK(t, err)

	data := rtest.Random(23, rand.Intn(MiB)+500*KiB)
	id := restic.Hash(data)
	// damage buffer
	data[0] ^= 0xff

	b.OpenReaderFn = func(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}

	// must fail on first try
	_, err = repo.LoadRaw(context.TODO(), backend.PackFile, id)
	rtest.Assert(t, errors.Is(err, restic.ErrInvalidData), "missing expected ErrInvalidData error, got %v", err)

	// must return the broken data after a retry
	be := &quickRetryBackend{Backend: b}
	repo, err = repository.New(be, repository.Options{})
	rtest.OK(t, err)
	buf, err := repo.LoadRaw(context.TODO(), backend.PackFile, id)
	rtest.Assert(t, errors.Is(err, restic.ErrInvalidData), "missing expected ErrInvalidData error, got %v", err)

	if !bytes.Equal(buf, data) {
		t.Fatalf("wrong data returned")
	}
}
