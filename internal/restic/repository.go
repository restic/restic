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
	// Connections returns the maximum number of concurrent backend operations
	Connections() uint
	Config() Config
	Key() *crypto.Key

	LoadIndex(ctx context.Context, p *progress.Counter) error
	SetIndex(mi MasterIndex) error

	LookupBlob(t BlobType, id ID) []PackedBlob
	LookupBlobSize(t BlobType, id ID) (size uint, exists bool)

	// ListBlobs runs fn on all blobs known to the index. When the context is cancelled,
	// the index iteration returns immediately with ctx.Err(). This blocks any modification of the index.
	ListBlobs(ctx context.Context, fn func(PackedBlob)) error
	ListPacksFromIndex(ctx context.Context, packs IDSet) <-chan PackBlobs
	// ListPack returns the list of blobs saved in the pack id and the length of
	// the pack header.
	ListPack(ctx context.Context, id ID, packSize int64) (entries []Blob, hdrSize uint32, err error)

	LoadBlob(ctx context.Context, t BlobType, id ID, buf []byte) ([]byte, error)
	LoadBlobsFromPack(ctx context.Context, packID ID, blobs []Blob, handleBlobFn func(blob BlobHandle, buf []byte, err error) error) error

	// StartPackUploader start goroutines to upload new pack files. The errgroup
	// is used to immediately notify about an upload error. Flush() will also return
	// that error.
	StartPackUploader(ctx context.Context, wg *errgroup.Group)
	SaveBlob(ctx context.Context, t BlobType, buf []byte, id ID, storeDuplicate bool) (newID ID, known bool, size int, err error)
	Flush(ctx context.Context) error

	// List calls the function fn for each file of type t in the repository.
	// When an error is returned by fn, processing stops and List() returns the
	// error.
	//
	// The function fn is called in the same Goroutine List() was called from.
	List(ctx context.Context, t FileType, fn func(ID, int64) error) error
	// LoadRaw reads all data stored in the backend for the file with id and filetype t.
	// If the backend returns data that does not match the id, then the buffer is returned
	// along with an error that is a restic.ErrInvalidData error.
	LoadRaw(ctx context.Context, t FileType, id ID) (data []byte, err error)
	// LoadUnpacked loads and decrypts the file with the given type and ID.
	LoadUnpacked(ctx context.Context, t FileType, id ID) (data []byte, err error)
	SaveUnpacked(ctx context.Context, t FileType, buf []byte) (ID, error)
	// RemoveUnpacked removes a file from the repository. This will eventually be restricted to deleting only snapshots.
	RemoveUnpacked(ctx context.Context, t FileType, id ID) error
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
	SaveUnpacked(ctx context.Context, t FileType, buf []byte) (ID, error)
}

// RemoverUnpacked allows removing an unpacked blob
type RemoverUnpacked interface {
	// Connections returns the maximum number of concurrent backend operations
	Connections() uint
	RemoveUnpacked(ctx context.Context, t FileType, id ID) error
}

type SaverRemoverUnpacked interface {
	SaverUnpacked
	RemoverUnpacked
}

type PackBlobs struct {
	PackID ID
	Blobs  []Blob
}

// MasterIndex keeps track of the blobs are stored within files.
type MasterIndex interface {
	Has(bh BlobHandle) bool
	Lookup(bh BlobHandle) []PackedBlob

	// Each runs fn on all blobs known to the index. When the context is cancelled,
	// the index iteration returns immediately with ctx.Err(). This blocks any modification of the index.
	Each(ctx context.Context, fn func(PackedBlob)) error
	ListPacks(ctx context.Context, packs IDSet) <-chan PackBlobs
}

// Lister allows listing files in a backend.
type Lister interface {
	List(ctx context.Context, t FileType, fn func(ID, int64) error) error
}

type ListerLoaderUnpacked interface {
	Lister
	LoaderUnpacked
}

type Unpacked interface {
	ListerLoaderUnpacked
	SaverUnpacked
	RemoverUnpacked
}

type ListBlobser interface {
	ListBlobs(ctx context.Context, fn func(PackedBlob)) error
}
