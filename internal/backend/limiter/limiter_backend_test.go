package limiter

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/mock"
	rtest "github.com/restic/restic/internal/test"
)

func randomBytes(t *testing.T, size int) []byte {
	data := make([]byte, size)
	_, err := io.ReadFull(rand.Reader, data)
	rtest.OK(t, err)
	return data
}

func TestLimitBackendSave(t *testing.T) {
	testHandle := backend.Handle{Type: backend.PackFile, Name: "test"}
	data := randomBytes(t, 1234)

	be := mock.NewBackend()
	be.SaveFn = func(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
		buf := new(bytes.Buffer)
		_, err := io.Copy(buf, rd)
		if err != nil {
			return nil
		}
		if !bytes.Equal(data, buf.Bytes()) {
			return fmt.Errorf("data mismatch")
		}
		return nil
	}
	limiter := NewStaticLimiter(Limits{42 * 1024, 42 * 1024})
	limbe := LimitBackend(be, limiter)

	rd := backend.NewByteReader(data, nil)
	err := limbe.Save(context.TODO(), testHandle, rd)
	rtest.OK(t, err)
}

type tracedReadWriteToCloser struct {
	io.Reader
	io.WriterTo
	Traced bool
}

func newTracedReadWriteToCloser(rd *bytes.Reader) *tracedReadWriteToCloser {
	return &tracedReadWriteToCloser{Reader: rd, WriterTo: rd}
}

func (r *tracedReadWriteToCloser) WriteTo(w io.Writer) (n int64, err error) {
	r.Traced = true
	return r.WriterTo.WriteTo(w)
}

func (r *tracedReadWriteToCloser) Close() error {
	return nil
}

func TestLimitBackendLoad(t *testing.T) {
	testHandle := backend.Handle{Type: backend.PackFile, Name: "test"}
	data := randomBytes(t, 1234)

	for _, test := range []struct {
		innerWriteTo, outerWriteTo bool
	}{{false, false}, {false, true}, {true, false}, {true, true}} {
		be := mock.NewBackend()
		src := newTracedReadWriteToCloser(bytes.NewReader(data))
		be.OpenReaderFn = func(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
			if length != 0 || offset != 0 {
				return nil, fmt.Errorf("Not supported")
			}
			// test both code paths in WriteTo of limitedReadCloser
			if test.innerWriteTo {
				return src, nil
			}
			return newTracedReadCloser(src), nil
		}
		limiter := NewStaticLimiter(Limits{42 * 1024, 42 * 1024})
		limbe := LimitBackend(be, limiter)

		err := limbe.Load(context.TODO(), testHandle, 0, 0, func(rd io.Reader) error {
			dataRead := new(bytes.Buffer)
			// test both Read and WriteTo
			if !test.outerWriteTo {
				rd = newTracedReadCloser(rd)
			}
			_, err := io.Copy(dataRead, rd)
			if err != nil {
				return err
			}
			if !bytes.Equal(data, dataRead.Bytes()) {
				return fmt.Errorf("read broken data")
			}

			return nil
		})
		rtest.OK(t, err)
		rtest.Assert(t, src.Traced == (test.innerWriteTo && test.outerWriteTo),
			"unexpected/missing writeTo call innerWriteTo %v outerWriteTo %v",
			test.innerWriteTo, test.outerWriteTo)
	}
}
