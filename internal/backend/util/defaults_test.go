package util_test

import (
	"context"
	"io"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/util"
	"github.com/restic/restic/internal/errors"

	rtest "github.com/restic/restic/internal/test"
)

type mockReader struct {
	closed bool
}

func (rd *mockReader) Read(_ []byte) (n int, err error) {
	return 0, nil
}
func (rd *mockReader) Close() error {
	rd.closed = true
	return nil
}

func TestDefaultLoad(t *testing.T) {

	h := backend.Handle{Name: "id", Type: backend.PackFile}
	rd := &mockReader{}

	// happy case, assert correct parameters are passed around and content stream is closed
	err := util.DefaultLoad(context.TODO(), h, 10, 11, func(ctx context.Context, ih backend.Handle, length int, offset int64) (io.ReadCloser, error) {
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
	err = util.DefaultLoad(context.TODO(), h, 10, 11, func(ctx context.Context, ih backend.Handle, length int, offset int64) (io.ReadCloser, error) {
		return nil, errors.Errorf("producer error")
	}, func(ird io.Reader) error {
		t.Fatalf("unexpected consumer invocation")
		return nil
	})
	rtest.Equals(t, "producer error", err.Error())

	// unhappy case, assert consumer errors are handled correctly
	rd = &mockReader{}
	err = util.DefaultLoad(context.TODO(), h, 10, 11, func(ctx context.Context, ih backend.Handle, length int, offset int64) (io.ReadCloser, error) {
		return rd, nil
	}, func(ird io.Reader) error {
		return errors.Errorf("consumer error")
	})
	rtest.Equals(t, true, rd.closed)
	rtest.Equals(t, "consumer error", err.Error())
}
