package backend

import "io"

// Type is the type of a Blob.
type Type string

// These are the different data types a backend can store.
const (
	Data     Type = "data"
	Key           = "key"
	Lock          = "lock"
	Snapshot      = "snapshot"
	Index         = "index"
	Config        = "config"
)

// Backend is used to store and access data.
type Backend interface {
	// Location returns a string that describes the type and location of the
	// repository.
	Location() string

	// Create creates a new Blob. The data is available only after Finalize()
	// has been called on the returned Blob.
	Create() (Blob, error)

	// GetReader returns an io.ReadCloser for the Blob with the given name of
	// type t at offset and length.
	GetReader(t Type, name string, offset, length uint) (io.ReadCloser, error)

	// Test a boolean value whether a Blob with the name and type exists.
	Test(t Type, name string) (bool, error)

	// Remove removes a Blob with type t and name.
	Remove(t Type, name string) error

	// Close the backend
	Close() error

	Lister

	// Load returns the data stored in the backend for h at the given offset
	// and saves it in p. Load has the same semantics as io.ReaderAt.
	Load(h Handle, p []byte, off int64) (int, error)
}

// Lister implements listing data items stored in a backend.
type Lister interface {
	// List returns a channel that yields all names of blobs of type t in an
	// arbitrary order. A goroutine is started for this. If the channel done is
	// closed, sending stops.
	List(t Type, done <-chan struct{}) <-chan string
}

// Deleter are backends that allow to self-delete all content stored in them.
type Deleter interface {
	// Delete the complete repository.
	Delete() error
}

// Blob is old.
type Blob interface {
	io.Writer

	// Finalize moves the data blob to the final location for type and name.
	Finalize(t Type, name string) error

	// Size returns the number of bytes written to the backend so far.
	Size() uint
}
