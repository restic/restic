package restic

import (
	"context"

	"github.com/restic/restic/internal/crypto"
)

// Repository stores data in a backend. It provides high-level functions and
// transparently encrypts/decrypts data.
type Repository interface {

	// Backend returns the backend used by the repository
	Backend() Backend

	Key() *crypto.Key

	SetIndex(MasterIndex) error

	Index() MasterIndex
	SaveFullIndex(context.Context) error
	SaveIndex(context.Context) error
	LoadIndex(context.Context) error

	Config() Config

	LookupBlobSize(ID, BlobType) (uint, bool)

	// List calls the function fn for each file of type t in the repository.
	// When an error is returned by fn, processing stops and List() returns the
	// error.
	//
	// The function fn is called in the same Goroutine List() was called from.
	List(ctx context.Context, t FileType, fn func(ID, int64) error) error

	// ListPack returns the list of blobs saved in the pack id and the length of
	// the the pack header.
	ListPack(context.Context, ID, int64) ([]Blob, uint32, error)

	Flush(context.Context) error

	SaveUnpacked(context.Context, FileType, []byte) (ID, error)
	SaveJSONUnpacked(context.Context, FileType, interface{}) (ID, error)

	LoadJSONUnpacked(ctx context.Context, t FileType, id ID, dest interface{}) error
	// LoadAndDecrypt loads and decrypts the file with the given type and ID,
	// using the supplied buffer (which must be empty). If the buffer is nil, a
	// new buffer will be allocated and returned.
	LoadAndDecrypt(ctx context.Context, buf []byte, t FileType, id ID) (data []byte, err error)

	LoadBlob(context.Context, BlobType, ID, []byte) ([]byte, error)
	SaveBlob(context.Context, BlobType, []byte, ID, bool) (ID, bool, error)

	LoadTree(context.Context, ID) (*Tree, error)
	SaveTree(context.Context, *Tree) (ID, error)
}

// Lister allows listing files in a backend.
type Lister interface {
	List(context.Context, FileType, func(FileInfo) error) error
}

// MasterIndex keeps track of the blobs are stored within files.
type MasterIndex interface {
	Has(BlobHandle) bool
	Lookup(BlobHandle) []PackedBlob
	Count(BlobType) uint
	PackSize(ctx context.Context, onlyHdr bool) map[ID]int64

	// Each returns a channel that yields all blobs known to the index. When
	// the context is cancelled, the background goroutine terminates. This
	// blocks any modification of the index.
	Each(ctx context.Context) <-chan PackedBlob
}
