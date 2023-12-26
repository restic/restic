package retry

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/mock"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

func TestBackendSaveRetry(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	errcount := 0
	be := &mock.Backend{
		SaveFn: func(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
			if errcount == 0 {
				errcount++
				_, err := io.CopyN(io.Discard, rd, 120)
				if err != nil {
					return err
				}

				return errors.New("injected error")
			}

			_, err := io.Copy(buf, rd)
			return err
		},
	}

	TestFastRetries(t)
	retryBackend := New(be, 10, nil, nil)

	data := test.Random(23, 5*1024*1024+11241)
	err := retryBackend.Save(context.TODO(), backend.Handle{}, backend.NewByteReader(data, be.Hasher()))
	if err != nil {
		t.Fatal(err)
	}

	if len(data) != buf.Len() {
		t.Errorf("wrong number of bytes written: want %d, got %d", len(data), buf.Len())
	}

	if !bytes.Equal(data, buf.Bytes()) {
		t.Errorf("wrong data written to backend")
	}
}

func TestBackendSaveRetryAtomic(t *testing.T) {
	errcount := 0
	calledRemove := false
	be := &mock.Backend{
		SaveFn: func(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
			if errcount == 0 {
				errcount++
				return errors.New("injected error")
			}
			return nil
		},
		RemoveFn: func(ctx context.Context, h backend.Handle) error {
			calledRemove = true
			return nil
		},
		HasAtomicReplaceFn: func() bool { return true },
	}

	TestFastRetries(t)
	retryBackend := New(be, 10, nil, nil)

	data := test.Random(23, 5*1024*1024+11241)
	err := retryBackend.Save(context.TODO(), backend.Handle{}, backend.NewByteReader(data, be.Hasher()))
	if err != nil {
		t.Fatal(err)
	}
	if calledRemove {
		t.Fatal("remove must not be called")
	}
}

func TestBackendListRetry(t *testing.T) {
	const (
		ID1 = "id1"
		ID2 = "id2"
	)

	retry := 0
	be := &mock.Backend{
		ListFn: func(ctx context.Context, t backend.FileType, fn func(backend.FileInfo) error) error {
			// fail during first retry, succeed during second
			retry++
			if retry == 1 {
				_ = fn(backend.FileInfo{Name: ID1})
				return errors.New("test list error")
			}
			_ = fn(backend.FileInfo{Name: ID1})
			_ = fn(backend.FileInfo{Name: ID2})
			return nil
		},
	}

	TestFastRetries(t)
	retryBackend := New(be, 10, nil, nil)

	var listed []string
	err := retryBackend.List(context.TODO(), backend.PackFile, func(fi backend.FileInfo) error {
		listed = append(listed, fi.Name)
		return nil
	})
	test.OK(t, err)                            // assert overall success
	test.Equals(t, 2, retry)                   // assert retried once
	test.Equals(t, []string{ID1, ID2}, listed) // assert no duplicate files
}

func TestBackendListRetryErrorFn(t *testing.T) {
	var names = []string{"id1", "id2", "foo", "bar"}

	be := &mock.Backend{
		ListFn: func(ctx context.Context, tpe backend.FileType, fn func(backend.FileInfo) error) error {
			t.Logf("List called for %v", tpe)
			for _, name := range names {
				err := fn(backend.FileInfo{Name: name})
				if err != nil {
					return err
				}
			}

			return nil
		},
	}

	TestFastRetries(t)
	retryBackend := New(be, 10, nil, nil)

	var ErrTest = errors.New("test error")

	var listed []string
	run := 0
	err := retryBackend.List(context.TODO(), backend.PackFile, func(fi backend.FileInfo) error {
		t.Logf("fn called for %v", fi.Name)
		run++
		// return an error for the third item in the list
		if run == 3 {
			t.Log("returning an error")
			return ErrTest
		}
		listed = append(listed, fi.Name)
		return nil
	})

	if err != ErrTest {
		t.Fatalf("wrong error returned, want %v, got %v", ErrTest, err)
	}

	// processing should stop after the error was returned, so run should be 3
	if run != 3 {
		t.Fatalf("function was called %d times, wanted %v", run, 3)
	}

	test.Equals(t, []string{"id1", "id2"}, listed)
}

func TestBackendListRetryErrorBackend(t *testing.T) {
	var names = []string{"id1", "id2", "foo", "bar"}

	var ErrBackendTest = errors.New("test error")

	retries := 0
	be := &mock.Backend{
		ListFn: func(ctx context.Context, tpe backend.FileType, fn func(backend.FileInfo) error) error {
			t.Logf("List called for %v, retries %v", tpe, retries)
			retries++
			for i, name := range names {
				if i == 2 {
					return ErrBackendTest
				}

				err := fn(backend.FileInfo{Name: name})
				if err != nil {
					return err
				}
			}

			return nil
		},
	}

	TestFastRetries(t)
	const maxRetries = 2
	retryBackend := New(be, maxRetries, nil, nil)

	var listed []string
	err := retryBackend.List(context.TODO(), backend.PackFile, func(fi backend.FileInfo) error {
		t.Logf("fn called for %v", fi.Name)
		listed = append(listed, fi.Name)
		return nil
	})

	if err != ErrBackendTest {
		t.Fatalf("wrong error returned, want %v, got %v", ErrBackendTest, err)
	}

	if retries != maxRetries+1 {
		t.Fatalf("List was called %d times, wanted %v", retries, maxRetries+1)
	}

	test.Equals(t, names[:2], listed)
}

// failingReader returns an error after reading limit number of bytes
type failingReader struct {
	data  []byte
	pos   int
	limit int
}

func (r failingReader) Read(p []byte) (n int, err error) {
	i := 0
	for ; i < len(p) && i+r.pos < r.limit; i++ {
		p[i] = r.data[r.pos+i]
	}
	r.pos += i
	if r.pos >= r.limit {
		return i, errors.Errorf("reader reached limit of %d", r.limit)
	}
	return i, nil
}
func (r failingReader) Close() error {
	return nil
}

// closingReader adapts io.Reader to io.ReadCloser interface
type closingReader struct {
	rd io.Reader
}

func (r closingReader) Read(p []byte) (n int, err error) {
	return r.rd.Read(p)
}
func (r closingReader) Close() error {
	return nil
}

func TestBackendLoadRetry(t *testing.T) {
	data := test.Random(23, 1024)
	limit := 100
	attempt := 0

	be := mock.NewBackend()
	be.OpenReaderFn = func(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
		// returns failing reader on first invocation, good reader on subsequent invocations
		attempt++
		if attempt > 1 {
			return closingReader{rd: bytes.NewReader(data)}, nil
		}
		return failingReader{data: data, limit: limit}, nil
	}

	TestFastRetries(t)
	retryBackend := New(be, 10, nil, nil)

	var buf []byte
	err := retryBackend.Load(context.TODO(), backend.Handle{}, 0, 0, func(rd io.Reader) (err error) {
		buf, err = io.ReadAll(rd)
		return err
	})
	test.OK(t, err)
	test.Equals(t, data, buf)
	test.Equals(t, 2, attempt)
}

func TestBackendLoadNotExists(t *testing.T) {
	// load should not retry if the error matches IsNotExist
	notFound := errors.New("not found")
	attempt := 0

	be := mock.NewBackend()
	be.OpenReaderFn = func(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
		attempt++
		if attempt > 1 {
			t.Fail()
			return nil, errors.New("must not retry")
		}
		return nil, notFound
	}
	be.IsNotExistFn = func(err error) bool {
		return errors.Is(err, notFound)
	}

	TestFastRetries(t)
	retryBackend := New(be, 10, nil, nil)

	err := retryBackend.Load(context.TODO(), backend.Handle{}, 0, 0, func(rd io.Reader) (err error) {
		return nil
	})
	test.Assert(t, be.IsNotExistFn(err), "unexpected error %v", err)
	test.Equals(t, 1, attempt)
}

func TestBackendStatNotExists(t *testing.T) {
	// stat should not retry if the error matches IsNotExist
	notFound := errors.New("not found")
	attempt := 0

	be := mock.NewBackend()
	be.StatFn = func(ctx context.Context, h backend.Handle) (backend.FileInfo, error) {
		attempt++
		if attempt > 1 {
			t.Fail()
			return backend.FileInfo{}, errors.New("must not retry")
		}
		return backend.FileInfo{}, notFound
	}
	be.IsNotExistFn = func(err error) bool {
		return errors.Is(err, notFound)
	}

	TestFastRetries(t)
	retryBackend := New(be, 10, nil, nil)

	_, err := retryBackend.Stat(context.TODO(), backend.Handle{})
	test.Assert(t, be.IsNotExistFn(err), "unexpected error %v", err)
	test.Equals(t, 1, attempt)
}

func assertIsCanceled(t *testing.T, err error) {
	test.Assert(t, err == context.Canceled, "got unexpected err %v", err)
}

func TestBackendCanceledContext(t *testing.T) {
	// unimplemented mock backend functions return an error by default
	// check that we received the expected context canceled error instead
	TestFastRetries(t)
	retryBackend := New(mock.NewBackend(), 2, nil, nil)
	h := backend.Handle{Type: backend.PackFile, Name: restic.NewRandomID().String()}

	// create an already canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := retryBackend.Stat(ctx, h)
	assertIsCanceled(t, err)

	err = retryBackend.Save(ctx, h, backend.NewByteReader([]byte{}, nil))
	assertIsCanceled(t, err)
	err = retryBackend.Remove(ctx, h)
	assertIsCanceled(t, err)
	err = retryBackend.Load(ctx, backend.Handle{}, 0, 0, func(rd io.Reader) (err error) {
		return nil
	})
	assertIsCanceled(t, err)
	err = retryBackend.List(ctx, backend.PackFile, func(backend.FileInfo) error {
		return nil
	})
	assertIsCanceled(t, err)

	// don't test "Delete" as it is not used by normal code
}

func TestNotifyWithSuccessIsNotCalled(t *testing.T) {
	operation := func() error {
		return nil
	}

	notify := func(error, time.Duration) {
		t.Fatal("Notify should not have been called")
	}

	success := func(retries int) {
		t.Fatal("Success should not have been called")
	}

	err := retryNotifyErrorWithSuccess(operation, &backoff.ZeroBackOff{}, notify, success)
	if err != nil {
		t.Fatal("retry should not have returned an error")
	}
}

func TestNotifyWithSuccessIsCalled(t *testing.T) {
	operationCalled := 0
	operation := func() error {
		operationCalled++
		if operationCalled <= 2 {
			return errors.New("expected error in test")
		}
		return nil
	}

	notifyCalled := 0
	notify := func(error, time.Duration) {
		notifyCalled++
	}

	successCalled := 0
	success := func(retries int) {
		successCalled++
	}

	err := retryNotifyErrorWithSuccess(operation, &backoff.ZeroBackOff{}, notify, success)
	if err != nil {
		t.Fatal("retry should not have returned an error")
	}

	if notifyCalled != 2 {
		t.Fatalf("Notify should have been called 2 times, but was called %d times instead", notifyCalled)
	}

	if successCalled != 1 {
		t.Fatalf("Success should have been called only once, but was called %d times instead", successCalled)
	}
}
