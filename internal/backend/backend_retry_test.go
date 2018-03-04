package backend

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"testing"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/mock"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

func TestBackendSaveRetry(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	errcount := 0
	be := &mock.Backend{
		SaveFn: func(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
			if errcount == 0 {
				errcount++
				_, err := io.CopyN(ioutil.Discard, rd, 120)
				if err != nil {
					return err
				}

				return errors.New("injected error")
			}

			_, err := io.Copy(buf, rd)
			return err
		},
	}

	retryBackend := RetryBackend{
		Backend: be,
	}

	data := test.Random(23, 5*1024*1024+11241)
	err := retryBackend.Save(context.TODO(), restic.Handle{}, restic.NewByteReader(data))
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

func TestBackendListRetry(t *testing.T) {
	const (
		ID1 = "id1"
		ID2 = "id2"
	)

	retry := 0
	be := &mock.Backend{
		ListFn: func(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
			// fail during first retry, succeed during second
			retry++
			if retry == 1 {
				fn(restic.FileInfo{Name: ID1})
				return errors.New("test list error")
			}
			fn(restic.FileInfo{Name: ID1})
			fn(restic.FileInfo{Name: ID2})
			return nil
		},
	}

	retryBackend := RetryBackend{
		Backend: be,
	}

	var listed []string
	err := retryBackend.List(context.TODO(), restic.DataFile, func(fi restic.FileInfo) error {
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
		ListFn: func(ctx context.Context, tpe restic.FileType, fn func(restic.FileInfo) error) error {
			t.Logf("List called for %v", tpe)
			for _, name := range names {
				err := fn(restic.FileInfo{Name: name})
				if err != nil {
					return err
				}
			}

			return nil
		},
	}

	retryBackend := RetryBackend{
		Backend: be,
	}

	var ErrTest = errors.New("test error")

	var listed []string
	run := 0
	err := retryBackend.List(context.TODO(), restic.DataFile, func(fi restic.FileInfo) error {
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
		ListFn: func(ctx context.Context, tpe restic.FileType, fn func(restic.FileInfo) error) error {
			t.Logf("List called for %v, retries %v", tpe, retries)
			retries++
			for i, name := range names {
				if i == 2 {
					return ErrBackendTest
				}

				err := fn(restic.FileInfo{Name: name})
				if err != nil {
					return err
				}
			}

			return nil
		},
	}

	const maxRetries = 2
	retryBackend := RetryBackend{
		MaxTries: maxRetries,
		Backend:  be,
	}

	var listed []string
	err := retryBackend.List(context.TODO(), restic.DataFile, func(fi restic.FileInfo) error {
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
	be.OpenReaderFn = func(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
		// returns failing reader on first invocation, good reader on subsequent invocations
		attempt++
		if attempt > 1 {
			return closingReader{rd: bytes.NewReader(data)}, nil
		}
		return failingReader{data: data, limit: limit}, nil
	}

	retryBackend := RetryBackend{
		Backend: be,
	}

	var buf []byte
	err := retryBackend.Load(context.TODO(), restic.Handle{}, 0, 0, func(rd io.Reader) (err error) {
		buf, err = ioutil.ReadAll(rd)
		return err
	})
	test.OK(t, err)
	test.Equals(t, data, buf)
	test.Equals(t, 2, attempt)
}
