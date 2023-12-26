package backend_test

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/backend/mock"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

const KiB = 1 << 10
const MiB = 1 << 20

func TestLoadAll(t *testing.T) {
	b := mem.New()
	var buf []byte

	for i := 0; i < 20; i++ {
		data := rtest.Random(23+i, rand.Intn(MiB)+500*KiB)

		id := restic.Hash(data)
		h := backend.Handle{Name: id.String(), Type: backend.PackFile}
		err := b.Save(context.TODO(), h, backend.NewByteReader(data, b.Hasher()))
		rtest.OK(t, err)

		buf, err := backend.LoadAll(context.TODO(), buf, b, backend.Handle{Type: backend.PackFile, Name: id.String()})
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

func save(t testing.TB, be backend.Backend, buf []byte) backend.Handle {
	id := restic.Hash(buf)
	h := backend.Handle{Name: id.String(), Type: backend.PackFile}
	err := be.Save(context.TODO(), h, backend.NewByteReader(buf, be.Hasher()))
	if err != nil {
		t.Fatal(err)
	}
	return h
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

	data := rtest.Random(23, rand.Intn(MiB)+500*KiB)
	id := restic.Hash(data)
	// damage buffer
	data[0] ^= 0xff

	b.OpenReaderFn = func(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}

	// must fail on first try
	_, err := backend.LoadAll(context.TODO(), nil, b, backend.Handle{Type: backend.PackFile, Name: id.String()})
	if err == nil {
		t.Fatalf("missing expected error")
	}

	// must return the broken data after a retry
	be := &quickRetryBackend{Backend: b}
	buf, err := backend.LoadAll(context.TODO(), nil, be, backend.Handle{Type: backend.PackFile, Name: id.String()})
	rtest.OK(t, err)

	if !bytes.Equal(buf, data) {
		t.Fatalf("wrong data returned")
	}
}

func TestLoadAllAppend(t *testing.T) {
	b := mem.New()

	h1 := save(t, b, []byte("foobar test string"))
	randomData := rtest.Random(23, rand.Intn(MiB)+500*KiB)
	h2 := save(t, b, randomData)

	var tests = []struct {
		handle backend.Handle
		buf    []byte
		want   []byte
	}{
		{
			handle: h1,
			buf:    nil,
			want:   []byte("foobar test string"),
		},
		{
			handle: h1,
			buf:    []byte("xxx"),
			want:   []byte("foobar test string"),
		},
		{
			handle: h2,
			buf:    nil,
			want:   randomData,
		},
		{
			handle: h2,
			buf:    make([]byte, 0, 200),
			want:   randomData,
		},
		{
			handle: h2,
			buf:    []byte("foobarbaz"),
			want:   randomData,
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			buf, err := backend.LoadAll(context.TODO(), test.buf, b, test.handle)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(buf, test.want) {
				t.Errorf("wrong data returned, want %q, got %q", test.want, buf)
			}
		})
	}
}
