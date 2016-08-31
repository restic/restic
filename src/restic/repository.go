package restic

import "github.com/restic/chunker"

// Repository stores data in a backend. It provides high-level functions and
// transparently encrypts/decrypts data.
type Repository interface {

	// Backend returns the backend used by the repository
	Backend() Backend

	SetIndex(interface{})

	Index() Index
	SaveFullIndex() error

	SaveJSON(BlobType, interface{}) (ID, error)

	Config() Config

	SaveAndEncrypt(BlobType, []byte, *ID) (ID, error)
	SaveJSONUnpacked(FileType, interface{}) (ID, error)
	SaveIndex() error

	LoadJSONPack(BlobType, ID, interface{}) error
	LoadJSONUnpacked(FileType, ID, interface{}) error
	LoadBlob(ID, BlobType, []byte) ([]byte, error)

	LookupBlobSize(ID, BlobType) (uint, error)

	List(FileType, <-chan struct{}) <-chan ID
	ListPack(ID) ([]Blob, int64, error)

	Flush() error
}

// Index keeps track of the blobs are stored within files.
type Index interface {
	Has(ID, BlobType) bool
	Lookup(ID, BlobType) ([]PackedBlob, error)
}

// Config stores information about the repository.
type Config interface {
	ChunkerPolynomial() chunker.Pol
}
