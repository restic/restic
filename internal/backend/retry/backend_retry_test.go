package retry

import (
	"bytes"
	"context"
	"io"
	"strings"
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
	const maxElapsedTime = 10 * time.Millisecond
	now := time.Now()
	retryBackend := New(be, maxElapsedTime, nil, nil)

	var listed []string
	err := retryBackend.List(context.TODO(), backend.PackFile, func(fi backend.FileInfo) error {
		t.Logf("fn called for %v", fi.Name)
		listed = append(listed, fi.Name)
		return nil
	})

	if err != ErrBackendTest {
		t.Fatalf("wrong error returned, want %v, got %v", ErrBackendTest, err)
	}

	duration := time.Since(now)
	if duration > 100*time.Millisecond {
		t.Fatalf("list retries took %v, expected at most 10ms", duration)
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
	be.IsPermanentErrorFn = func(err error) bool {
		return errors.Is(err, notFound)
	}

	TestFastRetries(t)
	retryBackend := New(be, 10, nil, nil)

	err := retryBackend.Load(context.TODO(), backend.Handle{}, 0, 0, func(rd io.Reader) (err error) {
		return nil
	})
	test.Assert(t, be.IsPermanentErrorFn(err), "unexpected error %v", err)
	test.Equals(t, 1, attempt)
}

func TestBackendLoadCircuitBreaker(t *testing.T) {
	// retry should not retry if the error matches IsPermanentError
	notFound := errors.New("not found")
	otherError := errors.New("something")
	attempt := 0

	be := mock.NewBackend()
	be.IsPermanentErrorFn = func(err error) bool {
		return errors.Is(err, notFound)
	}
	be.OpenReaderFn = func(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
		attempt++
		return nil, otherError
	}
	nilRd := func(rd io.Reader) (err error) {
		return nil
	}

	TestFastRetries(t)
	retryBackend := New(be, 2, nil, nil)
	// trip the circuit breaker for file "other"
	err := retryBackend.Load(context.TODO(), backend.Handle{Name: "other"}, 0, 0, nilRd)
	test.Equals(t, otherError, err, "unexpected error")
	test.Equals(t, 2, attempt)

	attempt = 0
	err = retryBackend.Load(context.TODO(), backend.Handle{Name: "other"}, 0, 0, nilRd)
	test.Assert(t, strings.Contains(err.Error(), "circuit breaker open for file"), "expected circuit breaker error, got %v")
	test.Equals(t, 0, attempt)

	// don't trip for permanent errors
	be.OpenReaderFn = func(ctx context.Context, h backend.Handle, length int, offset int64) (io.ReadCloser, error) {
		attempt++
		return nil, notFound
	}
	err = retryBackend.Load(context.TODO(), backend.Handle{Name: "notfound"}, 0, 0, nilRd)
	test.Equals(t, notFound, err, "expected circuit breaker to only affect other file, got %v")
	err = retryBackend.Load(context.TODO(), backend.Handle{Name: "notfound"}, 0, 0, nilRd)
	test.Equals(t, notFound, err, "persistent error must not trigger circuit breaker, got %v")

	// wait for circuit breaker to expire
	time.Sleep(5 * time.Millisecond)
	old := failedLoadExpiry
	defer func() {
		failedLoadExpiry = old
	}()
	failedLoadExpiry = 3 * time.Millisecond
	err = retryBackend.Load(context.TODO(), backend.Handle{Name: "other"}, 0, 0, nilRd)
	test.Equals(t, notFound, err, "expected circuit breaker to reset, got %v")
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

func TestBackendRetryPermanent(t *testing.T) {
	// retry should not retry if the error matches IsPermanentError
	notFound := errors.New("not found")
	attempt := 0

	be := mock.NewBackend()
	be.IsPermanentErrorFn = func(err error) bool {
		return errors.Is(err, notFound)
	}

	TestFastRetries(t)
	retryBackend := New(be, 2, nil, nil)
	err := retryBackend.retry(context.TODO(), "test", func() error {
		attempt++
		return notFound
	})

	test.Assert(t, be.IsPermanentErrorFn(err), "unexpected error %v", err)
	test.Equals(t, 1, attempt)

	attempt = 0
	err = retryBackend.retry(context.TODO(), "test", func() error {
		attempt++
		return errors.New("something")
	})
	test.Assert(t, !be.IsPermanentErrorFn(err), "error unexpectedly considered permanent %v", err)
	test.Equals(t, 2, attempt)

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

	err := retryNotifyErrorWithSuccess(operation, backoff.WithContext(&backoff.ZeroBackOff{}, context.Background()), notify, success)
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

	err := retryNotifyErrorWithSuccess(operation, backoff.WithContext(&backoff.ZeroBackOff{}, context.Background()), notify, success)
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

func TestNotifyWithSuccessFinalError(t *testing.T) {
	operation := func() error {
		return errors.New("expected error in test")
	}

	notifyCalled := 0
	notify := func(error, time.Duration) {
		notifyCalled++
	}

	successCalled := 0
	success := func(retries int) {
		successCalled++
	}

	err := retryNotifyErrorWithSuccess(operation, backoff.WithContext(backoff.WithMaxRetries(&backoff.ZeroBackOff{}, 5), context.Background()), notify, success)
	test.Assert(t, err.Error() == "expected error in test", "wrong error message %v", err)
	test.Equals(t, 6, notifyCalled, "notify should have been called 6 times")
	test.Equals(t, 0, successCalled, "success should not have been called")
}

func TestNotifyWithCancelError(t *testing.T) {
	operation := func() error {
		return errors.New("expected error in test")
	}

	notify := func(error, time.Duration) {
		t.Error("unexpected call to notify")
	}

	success := func(retries int) {
		t.Error("unexpected call to success")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := retryNotifyErrorWithSuccess(operation, backoff.WithContext(&backoff.ZeroBackOff{}, ctx), notify, success)
	test.Assert(t, err == context.Canceled, "wrong error message %v", err)
}

type testClock struct {
	Time time.Time
}

func (c *testClock) Now() time.Time {
	return c.Time
}

func TestRetryAtLeastOnce(t *testing.T) {
	expBackOff := backoff.NewExponentialBackOff()
	expBackOff.InitialInterval = 500 * time.Millisecond
	expBackOff.RandomizationFactor = 0
	expBackOff.MaxElapsedTime = 5 * time.Second
	expBackOff.Multiplier = 2 // guarantee numerical stability
	clock := &testClock{Time: time.Now()}
	expBackOff.Clock = clock
	expBackOff.Reset()

	retry := withRetryAtLeastOnce(expBackOff)

	// expire backoff
	clock.Time = clock.Time.Add(10 * time.Second)
	delay := retry.NextBackOff()
	test.Equals(t, expBackOff.InitialInterval, delay, "must retry at least once")

	delay = retry.NextBackOff()
	test.Equals(t, expBackOff.Stop, delay, "must not retry more than once")

	// test reset behavior
	retry.Reset()
	test.Equals(t, uint64(0), retry.numTries, "numTries should be reset to 0")

	// Verify that after reset, NextBackOff returns the initial interval again
	delay = retry.NextBackOff()
	test.Equals(t, expBackOff.InitialInterval, delay, "retries must work after reset")

	delay = retry.NextBackOff()
	test.Equals(t, expBackOff.InitialInterval*time.Duration(expBackOff.Multiplier), delay, "retries must work after reset")
}
