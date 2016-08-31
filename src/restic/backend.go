package restic

// FileType is the type of a file in the backend.
type FileType string

// These are the different data types a backend can store.
const (
	DataFile     FileType = "data"
	KeyFile               = "key"
	LockFile              = "lock"
	SnapshotFile          = "snapshot"
	IndexFile             = "index"
	ConfigFile            = "config"
)

// Backend is used to store and access data.
type Backend interface {
	// Location returns a string that describes the type and location of the
	// repository.
	Location() string

	// Test a boolean value whether a Blob with the name and type exists.
	Test(t FileType, name string) (bool, error)

	// Remove removes a Blob with type t and name.
	Remove(t FileType, name string) error

	// Close the backend
	Close() error

	// Load returns the data stored in the backend for h at the given offset
	// and saves it in p. Load has the same semantics as io.ReaderAt, except
	// that a negative offset is also allowed. In this case it references a
	// position relative to the end of the file (similar to Seek()).
	Load(h Handle, p []byte, off int64) (int, error)

	// Save stores the data in the backend under the given handle.
	Save(h Handle, p []byte) error

	// Stat returns information about the blob identified by h.
	Stat(h Handle) (BlobInfo, error)

	// List returns a channel that yields all names of blobs of type t in an
	// arbitrary order. A goroutine is started for this. If the channel done is
	// closed, sending stops.
	List(t FileType, done <-chan struct{}) <-chan string

	// Delete the complete repository.
	Delete() error
}

// BlobInfo is returned by Stat() and contains information about a stored blob.
type BlobInfo struct {
	Size int64
}
