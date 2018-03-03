package backend

import (
	"context"
	"io"
	"io/ioutil"
	"math/rand"
	"sync"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
)

// ErrorBackend is used to induce errors into various function calls and test
// the retry functions.
type ErrorBackend struct {
	FailSave     float32
	FailSaveRead float32
	FailLoad     float32
	FailStat     float32
	restic.Backend

	r *rand.Rand
	m sync.Mutex
}

// statically ensure that ErrorBackend implements restic.Backend.
var _ restic.Backend = &ErrorBackend{}

// NewErrorBackend wraps be with a backend that returns errors according to
// given probabilities.
func NewErrorBackend(be restic.Backend, seed int64) *ErrorBackend {
	return &ErrorBackend{
		Backend: be,
		r:       rand.New(rand.NewSource(seed)),
	}
}

func (be *ErrorBackend) fail(p float32) bool {
	be.m.Lock()
	v := be.r.Float32()
	be.m.Unlock()

	return v < p
}

// Save stores the data in the backend under the given handle.
func (be *ErrorBackend) Save(ctx context.Context, h restic.Handle, rd restic.RewindReader) error {
	if be.fail(be.FailSave) {
		return errors.Errorf("Save(%v) random error induced", h)
	}

	if be.fail(be.FailSaveRead) {
		_, err := io.CopyN(ioutil.Discard, rd, be.r.Int63n(1000))
		if err != nil {
			return err
		}

		return errors.Errorf("Save(%v) random error with partial read induced", h)
	}

	return be.Backend.Save(ctx, h, rd)
}

// Load returns a reader that yields the contents of the file at h at the
// given offset. If length is larger than zero, only a portion of the file
// is returned. rd must be closed after use. If an error is returned, the
// ReadCloser must be nil.
func (be *ErrorBackend) Load(ctx context.Context, h restic.Handle, length int, offset int64, consumer func(rd io.Reader) error) error {
	if be.fail(be.FailLoad) {
		return errors.Errorf("Load(%v, %v, %v) random error induced", h, length, offset)
	}

	return be.Backend.Load(ctx, h, length, offset, consumer)
}

// Stat returns information about the File identified by h.
func (be *ErrorBackend) Stat(ctx context.Context, h restic.Handle) (restic.FileInfo, error) {
	if be.fail(be.FailLoad) {
		return restic.FileInfo{}, errors.Errorf("Stat(%v) random error induced", h)
	}

	return be.Stat(ctx, h)
}
