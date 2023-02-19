package retry

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

// Backend retries operations on the backend in case of an error with a
// backoff.
type Backend struct {
	restic.Backend
	MaxTries int
	Report   func(string, error, time.Duration)
	Success  func(string, int)
}

// statically ensure that RetryBackend implements restic.Backend.
var _ restic.Backend = &Backend{}

// New wraps be with a backend that retries operations after a
// backoff. report is called with a description and the error, if one occurred.
// success is called with the number of retries before a successful operation
// (it is not called if it succeeded on the first try)
func New(be restic.Backend, maxTries int, report func(string, error, time.Duration), success func(string, int)) *Backend {
	return &Backend{
		Backend:  be,
		MaxTries: maxTries,
		Report:   report,
		Success:  success,
	}
}

// retryNotifyErrorWithSuccess is an extension of backoff.RetryNotify with notification of success after an error.
// success is NOT notified on the first run of operation (only after an error).
func retryNotifyErrorWithSuccess(operation backoff.Operation, b backoff.BackOff, notify backoff.Notify, success func(retries int)) error {
	if success == nil {
		return backoff.RetryNotify(operation, b, notify)
	}
	retries := 0
	operationWrapper := func() error {
		err := operation()
		if err != nil {
			retries++
		} else if retries > 0 {
			success(retries)
		}
		return err
	}
	return backoff.RetryNotify(operationWrapper, b, notify)
}

var fastRetries = false

func (be *Backend) retry(ctx context.Context, msg string, f func() error) error {
	// Don't do anything when called with an already cancelled context. There would be
	// no retries in that case either, so be consistent and abort always.
	// This enforces a strict contract for backend methods: Using a cancelled context
	// will prevent any backup repository modifications. This simplifies ensuring that
	// a backup repository is not modified any further after a context was cancelled.
	// The 'local' backend for example does not provide this guarantee on its own.
	if ctx.Err() != nil {
		return ctx.Err()
	}

	bo := backoff.NewExponentialBackOff()
	if fastRetries {
		// speed up integration tests
		bo.InitialInterval = 1 * time.Millisecond
	}

	err := retryNotifyErrorWithSuccess(f,
		backoff.WithContext(backoff.WithMaxRetries(bo, uint64(be.MaxTries)), ctx),
		func(err error, d time.Duration) {
			if be.Report != nil {
				be.Report(msg, err, d)
			}
		},
		func(retries int) {
			if be.Success != nil {
				be.Success(msg, retries)
			}
		},
	)

	return err
}

// Save stores the data in the backend under the given handle.
func (be *Backend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	return be.retry(ctx, fmt.Sprintf("Save(%v)", h), func() error {
		err := rd.Rewind()
		if err != nil {
			return err
		}

		err = be.Backend.Save(ctx, h, rd)
		if err == nil {
			return nil
		}

		if be.Backend.HasAtomicReplace() {
			debug.Log("Save(%v) failed with error: %v", h, err)
			// there is no need to remove files from backends which can atomically replace files
			// in fact if something goes wrong at the backend side the delete operation might delete the wrong instance of the file
		} else {
			debug.Log("Save(%v) failed with error, removing file: %v", h, err)
			rerr := be.Backend.Remove(ctx, h)
			if rerr != nil {
				debug.Log("Remove(%v) returned error: %v", h, err)
			}
		}

		// return original error
		return err
	})
}

// Load returns a reader that yields the contents of the file at h at the
// given offset. If length is larger than zero, only a portion of the file
// is returned. rd must be closed after use. If an error is returned, the
// ReadCloser must be nil.
func (be *Backend) Load(ctx context.Context, h restic.Handle, length int, offset int64, consumer func(rd io.Reader) error) (err error) {
	return be.retry(ctx, fmt.Sprintf("Load(%v, %v, %v)", h, length, offset),
		func() error {
			return be.Backend.Load(ctx, h, length, offset, consumer)
		})
}

// Stat returns information about the File identified by h.
func (be *Backend) Stat(ctx context.Context, h restic.Handle) (fi restic.FileInfo, err error) {
	err = be.retry(ctx, fmt.Sprintf("Stat(%v)", h),
		func() error {
			var innerError error
			fi, innerError = be.Backend.Stat(ctx, h)

			if be.Backend.IsNotExist(innerError) {
				// do not retry if file is not found, as stat is usually used  to check whether a file exists
				return backoff.Permanent(innerError)
			}
			return innerError
		})
	return fi, err
}

// Remove removes a File with type t and name.
func (be *Backend) Remove(ctx context.Context, h restic.Handle) (err error) {
	return be.retry(ctx, fmt.Sprintf("Remove(%v)", h), func() error {
		return be.Backend.Remove(ctx, h)
	})
}

// List runs fn for each file in the backend which has the type t. When an
// error is returned by the underlying backend, the request is retried. When fn
// returns an error, the operation is aborted and the error is returned to the
// caller.
func (be *Backend) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
	// create a new context that we can cancel when fn returns an error, so
	// that listing is aborted
	listCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	listed := make(map[string]struct{}) // remember for which files we already ran fn
	var innerErr error                  // remember when fn returned an error, so we can return that to the caller

	err := be.retry(listCtx, fmt.Sprintf("List(%v)", t), func() error {
		return be.Backend.List(ctx, t, func(fi restic.FileInfo) error {
			if _, ok := listed[fi.Name]; ok {
				return nil
			}
			listed[fi.Name] = struct{}{}

			innerErr = fn(fi)
			if innerErr != nil {
				// if fn returned an error, listing is aborted, so we cancel the context
				cancel()
			}
			return innerErr
		})
	})

	// the error fn returned takes precedence
	if innerErr != nil {
		return innerErr
	}

	return err
}
