package limiter

import (
	"context"
	"io"

	"github.com/restic/restic/internal/restic"
)

// LimitBackend wraps a Backend and applies rate limiting to Load() and Save()
// calls on the backend.
func LimitBackend(be restic.Backend, l Limiter) restic.Backend {
	return rateLimitedBackend{
		Backend: be,
		limiter: l,
	}
}

type rateLimitedBackend struct {
	restic.Backend
	limiter Limiter
}

func (r rateLimitedBackend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	limited := limitedRewindReader{
		RewindReader: rd,
		limited:      r.limiter.Upstream(rd),
	}

	return r.Backend.Save(ctx, h, limited)
}

type limitedRewindReader struct {
	restic.RewindReader

	limited io.Reader
}

func (l limitedRewindReader) Read(b []byte) (int, error) {
	return l.limited.Read(b)
}

func (r rateLimitedBackend) Load(ctx context.Context, h restic.Handle, length int, offset int64, consumer func(rd io.Reader) error) error {
	return r.Backend.Load(ctx, h, length, offset, func(rd io.Reader) error {
		lrd := limitedReadCloser{
			limited: r.limiter.Downstream(rd),
		}
		return consumer(lrd)
	})
}

type limitedReadCloser struct {
	original io.ReadCloser
	limited  io.Reader
}

func (l limitedReadCloser) Read(b []byte) (n int, err error) {
	return l.limited.Read(b)
}

func (l limitedReadCloser) Close() error {
	if l.original == nil {
		return nil
	}
	return l.original.Close()
}

var _ restic.Backend = (*rateLimitedBackend)(nil)
