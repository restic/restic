package pack

import (
	"errors"
	"restic/backend"
)

// Loader loads data from somewhere at a given offset. In contrast to
// io.ReaderAt, off may be negative, in which case it references a position
// relative to the end of the file (similar to Seek()).
type Loader interface {
	Load(p []byte, off int64) (int, error)
}

// BackendLoader creates a Loader from a Backend and a Handle.
type BackendLoader struct {
	Backend backend.Backend
	Handle  backend.Handle
}

// Load returns data at the given offset.
func (l BackendLoader) Load(p []byte, off int64) (int, error) {
	return l.Backend.Load(l.Handle, p, off)
}

// BufferLoader allows using a buffer as a Loader.
type BufferLoader []byte

// Load returns data at the given offset.
func (b BufferLoader) Load(p []byte, off int64) (int, error) {
	switch {
	case off > int64(len(b)):
		return 0, errors.New("offset is larger than data")
	case off < -int64(len(b)):
		return 0, errors.New("offset starts before the beginning of the data")
	case off < 0:
		off = int64(len(b)) + off
	}

	b = b[off:]

	return copy(p, b), nil
}
