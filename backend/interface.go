package backend

import (
	"errors"
	"io"
)

// Type is the type of a Blob.
type Type string

const (
	Data     Type = "data"
	Key           = "key"
	Lock          = "lock"
	Snapshot      = "snapshot"
	Index         = "index"
	Config        = "config"
)

func ParseType(s string) (Type, error) {
	switch s {
	case string(Data):
		return Data, nil
	case string(Key):
		return Key, nil
	case string(Lock):
		return Lock, nil
	case string(Snapshot):
		return Snapshot, nil
	case string(Index):
		return Index, nil
	case string(Config):
		return Config, nil
	}
	return "", errors.New("invalid type")
}

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

	Lister
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
