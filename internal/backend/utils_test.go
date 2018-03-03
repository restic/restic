package backend_test

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

const KiB = 1 << 10
const MiB = 1 << 20

func TestLoadAll(t *testing.T) {
	b := mem.New()

	for i := 0; i < 20; i++ {
		data := rtest.Random(23+i, rand.Intn(MiB)+500*KiB)

		id := restic.Hash(data)
		h := restic.Handle{Name: id.String(), Type: restic.DataFile}
		err := b.Save(context.TODO(), h, restic.NewByteReader(data))
		rtest.OK(t, err)

		buf, err := backend.LoadAll(context.TODO(), b, restic.Handle{Type: restic.DataFile, Name: id.String()})
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

func TestLoadSmallBuffer(t *testing.T) {
	b := mem.New()

	for i := 0; i < 20; i++ {
		data := rtest.Random(23+i, rand.Intn(MiB)+500*KiB)

		id := restic.Hash(data)
		h := restic.Handle{Name: id.String(), Type: restic.DataFile}
		err := b.Save(context.TODO(), h, restic.NewByteReader(data))
		rtest.OK(t, err)

		buf, err := backend.LoadAll(context.TODO(), b, restic.Handle{Type: restic.DataFile, Name: id.String()})
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

func TestLoadLargeBuffer(t *testing.T) {
	b := mem.New()

	for i := 0; i < 20; i++ {
		data := rtest.Random(23+i, rand.Intn(MiB)+500*KiB)

		id := restic.Hash(data)
		h := restic.Handle{Name: id.String(), Type: restic.DataFile}
		err := b.Save(context.TODO(), h, restic.NewByteReader(data))
		rtest.OK(t, err)

		buf, err := backend.LoadAll(context.TODO(), b, restic.Handle{Type: restic.DataFile, Name: id.String()})
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

	h := restic.Handle{Name: "id", Type: restic.DataFile}
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
