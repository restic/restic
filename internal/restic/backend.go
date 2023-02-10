package restic

import (
	"context"
	"hash"
	"io"
)

// Backend is used to store and access data.
//
// Backend operations that return an error will be retried when a Backend is
// wrapped in a RetryBackend. To prevent that from happening, the operations
// should return a github.com/cenkalti/backoff/v4.PermanentError. Errors from
// the context package need not be wrapped, as context cancellation is checked
// separately by the retrying logic.
type Backend interface {
	// Location returns a string that describes the type and location of the
	// repository.
	Location() string

	// Connections returns the maxmimum number of concurrent backend operations.
	Connections() uint

	// Hasher may return a hash function for calculating a content hash for the backend
	Hasher() hash.Hash

	// HasAtomicReplace returns whether Save() can atomically replace files
	HasAtomicReplace() bool

	// Remove removes a File described  by h.
	Remove(ctx context.Context, h Handle) error

	// Close the backend
	Close() error

	// Save stores the data from rd under the given handle.
	Save(ctx context.Context, h Handle, rd RewindReader) error

	// Load runs fn with a reader that yields the contents of the file at h at the
	// given offset. If length is larger than zero, only a portion of the file
	// is read.
	//
	// The function fn may be called multiple times during the same Load invocation
	// and therefore must be idempotent.
	//
	// Implementations are encouraged to use backend.DefaultLoad
	Load(ctx context.Context, h Handle, length int, offset int64, fn func(rd io.Reader) error) error

	// Stat returns information about the File identified by h.
	Stat(ctx context.Context, h Handle) (FileInfo, error)

	// List runs fn for each file in the backend which has the type t. When an
	// error occurs (or fn returns an error), List stops and returns it.
	//
	// The function fn is called exactly once for each file during successful
	// execution and at most once in case of an error.
	//
	// The function fn is called in the same Goroutine that List() is called
	// from.
	List(ctx context.Context, t FileType, fn func(FileInfo) error) error

	// IsNotExist returns true if the error was caused by a non-existing file
	// in the backend.
	//
	// The argument may be a wrapped error. The implementation is responsible
	// for unwrapping it.
	IsNotExist(err error) bool

	// Delete removes all data in the backend.
	Delete(ctx context.Context) error
}

// FileInfo is contains information about a file in the backend.
type FileInfo struct {
	Size int64
	Name string
}
