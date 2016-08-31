package restic

import (
	"restic/pack"

	"github.com/restic/chunker"
)

// Repository stores data in a backend. It provides high-level functions and
// transparently encrypts/decrypts data.
type Repository interface {

	// Backend returns the backend used by the repository
	Backend() Backend

	SetIndex(interface{})

	Index() Index
	SaveFullIndex() error

	SaveJSON(pack.BlobType, interface{}) (ID, error)

	Config() Config

	SaveAndEncrypt(pack.BlobType, []byte, *ID) (ID, error)
	SaveJSONUnpacked(FileType, interface{}) (ID, error)
	SaveIndex() error

	LoadJSONPack(pack.BlobType, ID, interface{}) error
	LoadJSONUnpacked(FileType, ID, interface{}) error
	LoadBlob(ID, pack.BlobType, []byte) ([]byte, error)

	LookupBlobSize(ID, pack.BlobType) (uint, error)

	List(FileType, <-chan struct{}) <-chan ID

	Flush() error
}

type Index interface {
	Has(ID, pack.BlobType) bool
	Lookup(ID, pack.BlobType) ([]PackedBlob, error)
}

type Config interface {
	ChunkerPolynomial() chunker.Pol
}

type PackedBlob interface {
	Type() pack.BlobType
	Length() uint
	ID() ID
	Offset() uint
	PackID() ID
}
