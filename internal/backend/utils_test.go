package backend_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/backend/mock"
	"github.com/restic/restic/internal/errors"
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
		h := restic.Handle{Name: id.String(), Type: restic.PackFile}
		err := b.Save(context.TODO(), h, restic.NewByteReader(data, b.Hasher()))
		rtest.OK(t, err)

		buf, err := backend.LoadAll(context.TODO(), buf, b, restic.Handle{Type: restic.PackFile, Name: id.String()})
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

func save(t testing.TB, be restic.Backend, buf []byte) restic.Handle {
	id := restic.Hash(buf)
	h := restic.Handle{Name: id.String(), Type: restic.PackFile}
	err := be.Save(context.TODO(), h, restic.NewByteReader(buf, be.Hasher()))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

type quickRetryBackend struct {
	restic.Backend
}

func (be *quickRetryBackend) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
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

	b.OpenReaderFn = func(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}

	// must fail on first try
	_, err := backend.LoadAll(context.TODO(), nil, b, restic.Handle{Type: restic.PackFile, Name: id.String()})
	if err == nil {
		t.Fatalf("missing expected error")
	}

	// must return the broken data after a retry
	be := &quickRetryBackend{Backend: b}
	buf, err := backend.LoadAll(context.TODO(), nil, be, restic.Handle{Type: restic.PackFile, Name: id.String()})
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
		handle restic.Handle
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

type mockReader struct {
	closed bool
}

func (rd *mockReader) Read(p []byte) (n int, err error) {
	return 0, nil
}
func (rd *mockReader) Close() error {
	rd.closed = true
	return nil
}

func TestDefaultLoad(t *testing.T) {

	h := restic.Handle{Name: "id", Type: restic.PackFile}
	rd := &mockReader{}

	// happy case, assert correct parameters are passed around and content stream is closed
	err := backend.DefaultLoad(context.TODO(), h, 10, 11, func(ctx context.Context, ih restic.Handle, length int, offset int64) (io.ReadCloser, error) {
		rtest.Equals(t, h, ih)
		rtest.Equals(t, int(10), length)
		rtest.Equals(t, int64(11), offset)

		return rd, nil
	}, func(ird io.Reader) error {
		rtest.Equals(t, rd, ird)
		return nil
	})
	rtest.OK(t, err)
	rtest.Equals(t, true, rd.closed)

	// unhappy case, assert producer errors are handled correctly
	err = backend.DefaultLoad(context.TODO(), h, 10, 11, func(ctx context.Context, ih restic.Handle, length int, offset int64) (io.ReadCloser, error) {
		return nil, errors.Errorf("producer error")
	}, func(ird io.Reader) error {
		t.Fatalf("unexpected consumer invocation")
		return nil
	})
	rtest.Equals(t, "producer error", err.Error())

	// unhappy case, assert consumer errors are handled correctly
	rd = &mockReader{}
	err = backend.DefaultLoad(context.TODO(), h, 10, 11, func(ctx context.Context, ih restic.Handle, length int, offset int64) (io.ReadCloser, error) {
		return rd, nil
	}, func(ird io.Reader) error {
		return errors.Errorf("consumer error")
	})
	rtest.Equals(t, true, rd.closed)
	rtest.Equals(t, "consumer error", err.Error())
}

func TestMemoizeList(t *testing.T) {
	// setup backend to serve as data source for memoized list
	be := mock.NewBackend()
	files := []restic.FileInfo{
		{Size: 42, Name: restic.NewRandomID().String()},
		{Size: 45, Name: restic.NewRandomID().String()},
	}
	be.ListFn = func(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
		for _, fi := range files {
			if err := fn(fi); err != nil {
				return err
			}
		}
		return nil
	}

	mem, err := backend.MemorizeList(context.TODO(), be, restic.SnapshotFile)
	rtest.OK(t, err)

	err = mem.List(context.TODO(), restic.IndexFile, func(fi restic.FileInfo) error {
		t.Fatal("file type mismatch")
		return nil // the memoized lister must return an error by itself
	})
	rtest.Assert(t, err != nil, "missing error on file typ mismatch")

	var memFiles []restic.FileInfo
	err = mem.List(context.TODO(), restic.SnapshotFile, func(fi restic.FileInfo) error {
		memFiles = append(memFiles, fi)
		return nil
	})
	rtest.OK(t, err)
	rtest.Equals(t, files, memFiles)
}

func TestMemoizeListError(t *testing.T) {
	// setup backend to serve as data source for memoized list
	be := mock.NewBackend()
	be.ListFn = func(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
		return fmt.Errorf("list error")
	}
	_, err := backend.MemorizeList(context.TODO(), be, restic.SnapshotFile)
	rtest.Assert(t, err != nil, "missing error on list error")
}
