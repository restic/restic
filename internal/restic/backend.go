package restic

import (
	"context"
	"io"
)

// Backend is used to store and access data.
type Backend interface {
	// Location returns a string that describes the type and location of the
	// repository.
	Location() string

	// Test a boolean value whether a File with the name and type exists.
	Test(ctx context.Context, h Handle) (bool, error)

	// Remove removes a File described  by h.
	Remove(ctx context.Context, h Handle) error

	// Close the backend
	Close() error

	// Save stores the data in the backend under the given handle.
	Save(ctx context.Context, h Handle, rd io.Reader) error

	// Load returns a reader that yields the contents of the file at h at the
	// given offset. If length is larger than zero, only a portion of the file
	// is returned. rd must be closed after use. If an error is returned, the
	// ReadCloser must be nil.
	Load(ctx context.Context, h Handle, length int, offset int64) (io.ReadCloser, error)

	// Stat returns information about the File identified by h.
	Stat(ctx context.Context, h Handle) (FileInfo, error)

	// List returns a channel that yields all names of files of type t in an
	// arbitrary order. A goroutine is started for this, which is stopped when
	// ctx is cancelled.
	List(ctx context.Context, t FileType) <-chan string

	// IsNotExist returns true if the error was caused by a non-existing file
	// in the backend.
	IsNotExist(err error) bool

	// Delete removes all data in the backend.
	Delete(ctx context.Context) error
}

// FileInfo is returned by Stat() and contains information about a file in the
// backend.
type FileInfo struct{ Size int64 }
