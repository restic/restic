package restic

import (
	"context"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/ui/progress"
	"golang.org/x/sync/errgroup"
)

// ErrInvalidData is used to report that a file is corrupted
var ErrInvalidData = errors.New("invalid data returned")

// Repository stores data in a backend. It provides high-level functions and
// transparently encrypts/decrypts data.
type Repository interface {

	// Backend returns the backend used by the repository
	Backend() backend.Backend
	// Connections returns the maximum number of concurrent backend operations
	Connections() uint

	Key() *crypto.Key

	Index() MasterIndex
	LoadIndex(context.Context, *progress.Counter) error
	ClearIndex()
	SetIndex(MasterIndex) error
	LookupBlobSize(ID, BlobType) (uint, bool)

	Config() Config
	PackSize() uint

	// List calls the function fn for each file of type t in the repository.
	// When an error is returned by fn, processing stops and List() returns the
	// error.
	//
	// The function fn is called in the same Goroutine List() was called from.
	List(ctx context.Context, t FileType, fn func(ID, int64) error) error

	// ListPack returns the list of blobs saved in the pack id and the length of
	// the pack header.
	ListPack(context.Context, ID, int64) ([]Blob, uint32, error)

	LoadBlob(context.Context, BlobType, ID, []byte) ([]byte, error)
	LoadBlobsFromPack(ctx context.Context, packID ID, blobs []Blob, handleBlobFn func(blob BlobHandle, buf []byte, err error) error) error
	SaveBlob(context.Context, BlobType, []byte, ID, bool) (ID, bool, int, error)

	// StartPackUploader start goroutines to upload new pack files. The errgroup
	// is used to immediately notify about an upload error. Flush() will also return
	// that error.
	StartPackUploader(ctx context.Context, wg *errgroup.Group)
	Flush(context.Context) error

	// LoadUnpacked loads and decrypts the file with the given type and ID.
	LoadUnpacked(ctx context.Context, t FileType, id ID) (data []byte, err error)
	SaveUnpacked(context.Context, FileType, []byte) (ID, error)
}

type FileType = backend.FileType

// These are the different data types a backend can store.
const (
	PackFile     FileType = backend.PackFile
	KeyFile      FileType = backend.KeyFile
	LockFile     FileType = backend.LockFile
	SnapshotFile FileType = backend.SnapshotFile
	IndexFile    FileType = backend.IndexFile
	ConfigFile   FileType = backend.ConfigFile
)

// LoaderUnpacked allows loading a blob not stored in a pack file
type LoaderUnpacked interface {
	// Connections returns the maximum number of concurrent backend operations
	Connections() uint
	LoadUnpacked(ctx context.Context, t FileType, id ID) (data []byte, err error)
}

// SaverUnpacked allows saving a blob not stored in a pack file
type SaverUnpacked interface {
	// Connections returns the maximum number of concurrent backend operations
	Connections() uint
	SaveUnpacked(context.Context, FileType, []byte) (ID, error)
}

type PackBlobs struct {
	PackID ID
	Blobs  []Blob
}

type MasterIndexSaveOpts struct {
	SaveProgress   *progress.Counter
	DeleteProgress func() *progress.Counter
	DeleteReport   func(id ID, err error)
	SkipDeletion   bool
}

// MasterIndex keeps track of the blobs are stored within files.
type MasterIndex interface {
	Has(BlobHandle) bool
	Lookup(BlobHandle) []PackedBlob

	// Each runs fn on all blobs known to the index. When the context is cancelled,
	// the index iteration return immediately. This blocks any modification of the index.
	Each(ctx context.Context, fn func(PackedBlob))
	ListPacks(ctx context.Context, packs IDSet) <-chan PackBlobs

	Save(ctx context.Context, repo Repository, excludePacks IDSet, extraObsolete IDs, opts MasterIndexSaveOpts) error
}

// Lister allows listing files in a backend.
type Lister interface {
	List(ctx context.Context, t FileType, fn func(ID, int64) error) error
}

type ListerLoaderUnpacked interface {
	Lister
	LoaderUnpacked
}
