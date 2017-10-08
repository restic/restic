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

func (r rateLimitedBackend) Save(ctx context.Context, h restic.Handle, rd io.Reader) error {
	return r.Backend.Save(ctx, h, r.limiter.Upstream(rd))
}

func (r rateLimitedBackend) Load(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error) {
	rc, err := r.Backend.Load(ctx, h, length, offset)
	if err != nil {
		return nil, err
	}

	return limitedReadCloser{
		original: rc,
		limited:  r.limiter.Downstream(rc),
	}, nil
}

type limitedReadCloser struct {
	original io.ReadCloser
	limited  io.Reader
}

func (l limitedReadCloser) Read(b []byte) (n int, err error) {
	return l.limited.Read(b)
}

func (l limitedReadCloser) Close() error {
	return l.original.Close()
}

var _ restic.Backend = (*rateLimitedBackend)(nil)
