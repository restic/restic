package backend

import "io"

// Type is the type of a Blob.
type Type string

const (
	Data     Type = "data"
	Key           = "key"
	Lock          = "lock"
	Snapshot      = "snapshot"
	Index         = "index"
)

const (
	Version = 1
)

// A Backend manages data stored somewhere.
type Backend interface {
	// Location returns a string that specifies the location of the repository,
	// like a URL.
	Location() string

	// Create creates a new Blob. The data is available only after Finalize()
	// has been called on the returned Blob.
	Create() (Blob, error)

	// Get returns an io.ReadCloser for the Blob with the given name of type t.
	Get(t Type, name string) (io.ReadCloser, error)

	// GetReader returns an io.ReadCloser for the Blob with the given name of
	// type t at offset and length.
	GetReader(t Type, name string, offset, length uint) (io.ReadCloser, error)

	// Test a boolean value whether a Blob with the name and type exists.
	Test(t Type, name string) (bool, error)

	// Remove removes a Blob with type t and name.
	Remove(t Type, name string) error

	// Close the backend
	Close() error

	Identifier
	Lister
}

type Identifier interface {
	// ID returns a unique ID for a specific repository. This means restic can
	// recognize repositories accessed via different methods (e.g. local file
	// access and sftp).
	ID() string
}

type Lister interface {
	// List returns a channel that yields all names of blobs of type t in
	// lexicographic order. A goroutine is started for this. If the channel
	// done is closed, sending stops.
	List(t Type, done <-chan struct{}) <-chan string
}

type Deleter interface {
	// Delete the complete repository.
	Delete() error
}

type Blob interface {
	io.Writer

	// Finalize moves the data blob to the final location for type and name.
	Finalize(t Type, name string) error

	// Size returns the number of bytes written to the backend so far.
	Size() uint
}
