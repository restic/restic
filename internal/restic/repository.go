package restic

import (
	"context"

	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/ui/progress"
)

// Repository stores data in a backend. It provides high-level functions and
// transparently encrypts/decrypts data.
type Repository interface {

	// Backend returns the backend used by the repository
	Backend() Backend

	Key() *crypto.Key

	Index() MasterIndex
	LoadIndex(context.Context) error
	SetIndex(MasterIndex) error
	LookupBlobSize(ID, BlobType) (uint, bool)

	Config() Config

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
	// LoadUnpacked loads and decrypts the file with the given type and ID,
	// using the supplied buffer (which must be empty). If the buffer is nil, a
	// new buffer will be allocated and returned.
	LoadUnpacked(ctx context.Context, buf []byte, t FileType, id ID) (data []byte, err error)

	LoadBlob(context.Context, BlobType, ID, []byte) ([]byte, error)
	SaveBlob(context.Context, BlobType, []byte, ID, bool) (ID, bool, error)

	LoadTree(context.Context, ID) (*Tree, error)
	SaveTree(context.Context, *Tree) (ID, error)
}

// Lister allows listing files in a backend.
type Lister interface {
	List(context.Context, FileType, func(FileInfo) error) error
}

// LoadJSONUnpackeder allows loading a JSON file not stored in a pack file
type LoadJSONUnpackeder interface {
	LoadJSONUnpacked(ctx context.Context, t FileType, id ID, dest interface{}) error
}

// SaverUnpacked allows saving a blob not stored in a pack file
type SaverUnpacked interface {
	SaveUnpacked(context.Context, FileType, []byte) (ID, error)
}

type PackBlobs struct {
	PackID ID
	Blobs  []Blob
}

// MasterIndex keeps track of the blobs are stored within files.
type MasterIndex interface {
	Has(BlobHandle) bool
	Lookup(BlobHandle) []PackedBlob

	// Each returns a channel that yields all blobs known to the index. When
	// the context is cancelled, the background goroutine terminates. This
	// blocks any modification of the index.
	Each(ctx context.Context) <-chan PackedBlob
	ListPacks(ctx context.Context, packs IDSet) <-chan PackBlobs

	Save(ctx context.Context, repo SaverUnpacked, packBlacklist IDSet, extraObsolete IDs, p *progress.Counter) (obsolete IDSet, err error)
}
