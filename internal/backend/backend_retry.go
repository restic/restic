package backend

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

// RetryBackend retries operations on the backend in case of an error with a
// backoff.
type RetryBackend struct {
	restic.Backend
	MaxTries int
	Report   func(string, error, time.Duration)
}

// statically ensure that RetryBackend implements restic.Backend.
var _ restic.Backend = &RetryBackend{}

// NewRetryBackend wraps be with a backend that retries operations after a
// backoff. report is called with a description and the error, if one occurred.
func NewRetryBackend(be restic.Backend, maxTries int, report func(string, error, time.Duration)) *RetryBackend {
	return &RetryBackend{
		Backend:  be,
		MaxTries: maxTries,
		Report:   report,
	}
}

func (be *RetryBackend) retry(ctx context.Context, msg string, f func() error) error {
	err := backoff.RetryNotify(f,
		backoff.WithContext(backoff.WithMaxTries(backoff.NewExponentialBackOff(), uint64(be.MaxTries)), ctx),
		func(err error, d time.Duration) {
			if be.Report != nil {
				be.Report(msg, err, d)
			}
		},
	)

	return err
}

// Save stores the data in the backend under the given handle.
func (be *RetryBackend) Save(ctx context.Context, h restic.Handle, rd io.Reader) error {
	seeker, ok := rd.(io.Seeker)
	if !ok {
		return errors.Errorf("reader %T is not a seeker", rd)
	}

	pos, err := seeker.Seek(0, io.SeekCurrent)
	if err != nil {
		return errors.Wrap(err, "Seek")
	}

	if pos != 0 {
		return errors.Errorf("reader is not at the beginning (pos %v)", pos)
	}

	return be.retry(ctx, fmt.Sprintf("Save(%v)", h), func() error {
		_, err := seeker.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}

		err = be.Backend.Save(ctx, h, rd)
		if err == nil {
			return nil
		}

		debug.Log("Save(%v) failed with error, removing file: %v", h, err)
		rerr := be.Backend.Remove(ctx, h)
		if rerr != nil {
			debug.Log("Remove(%v) returned error: %v", h, err)
		}

		// return original error
		return err
	})
}

// Load returns a reader that yields the contents of the file at h at the
// given offset. If length is larger than zero, only a portion of the file
// is returned. rd must be closed after use. If an error is returned, the
// ReadCloser must be nil.
func (be *RetryBackend) Load(ctx context.Context, h restic.Handle, length int, offset int64) (rd io.ReadCloser, err error) {
	err = be.retry(ctx, fmt.Sprintf("Load(%v, %v, %v)", h, length, offset),
		func() error {
			var innerError error
			rd, innerError = be.Backend.Load(ctx, h, length, offset)

			return innerError
		})
	return rd, err
}

// Stat returns information about the File identified by h.
func (be *RetryBackend) Stat(ctx context.Context, h restic.Handle) (fi restic.FileInfo, err error) {
	err = be.retry(ctx, fmt.Sprintf("Stat(%v)", h),
		func() error {
			var innerError error
			fi, innerError = be.Backend.Stat(ctx, h)

			return innerError
		})
	return fi, err
}

// Remove removes a File with type t and name.
func (be *RetryBackend) Remove(ctx context.Context, h restic.Handle) (err error) {
	return be.retry(ctx, fmt.Sprintf("Remove(%v)", h), func() error {
		return be.Backend.Remove(ctx, h)
	})
}

// Test a boolean value whether a File with the name and type exists.
func (be *RetryBackend) Test(ctx context.Context, h restic.Handle) (exists bool, err error) {
	err = be.retry(ctx, fmt.Sprintf("Test(%v)", h), func() error {
		var innerError error
		exists, innerError = be.Backend.Test(ctx, h)

		return innerError
	})
	return exists, err
}
