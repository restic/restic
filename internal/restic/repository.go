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

	SetIndex(Index) error

	Index() Index
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
	ListPack(context.Context, ID, int64) ([]Blob, int64, error)

	Flush(context.Context) error

	SaveUnpacked(context.Context, FileType, []byte) (ID, error)
	SaveJSONUnpacked(context.Context, FileType, interface{}) (ID, error)

	LoadJSONUnpacked(context.Context, FileType, ID, interface{}) error
	LoadAndDecrypt(context.Context, FileType, ID) ([]byte, error)

	LoadBlob(context.Context, BlobType, ID, []byte) (int, error)
	SaveBlob(context.Context, BlobType, []byte, ID) (ID, error)

	LoadTree(context.Context, ID) (*Tree, error)
	SaveTree(context.Context, *Tree) (ID, error)
}

// Lister allows listing files in a backend.
type Lister interface {
	List(context.Context, FileType, func(FileInfo) error) error
}

// Index keeps track of the blobs are stored within files.
type Index interface {
	Has(ID, BlobType) bool
	Lookup(ID, BlobType) ([]PackedBlob, bool)
	Count(BlobType) uint

	// Each returns a channel that yields all blobs known to the index. When
	// the context is cancelled, the background goroutine terminates. This
	// blocks any modification of the index.
	Each(ctx context.Context) <-chan PackedBlob
}
