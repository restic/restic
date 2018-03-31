package mock

import (
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/restic"
)

// Repository implements a mock Repository.
type Repository struct {
	BackendFn func() restic.Backend

	KeyFn func() *crypto.Key

	SetIndexFn func(restic.Index) error

	IndexFn         func() restic.Index
	SaveFullIndexFn func() error
	SaveIndexFn     func() error
	LoadIndexFn     func() error

	ConfigFn func() restic.Config

	LookupBlobSizeFn func(restic.ID, restic.BlobType) (uint, error)

	ListFn     func(restic.FileType, <-chan struct{}) <-chan restic.ID
	ListPackFn func(restic.ID) ([]restic.Blob, int64, error)

	FlushFn func() error

	SaveUnpackedFn     func(restic.FileType, []byte) (restic.ID, error)
	SaveJSONUnpackedFn func(restic.FileType, interface{}) (restic.ID, error)

	LoadJSONUnpackedFn func(restic.FileType, restic.ID, interface{}) error
	LoadAndDecryptFn   func(restic.FileType, restic.ID) ([]byte, error)

	LoadBlobFn func(restic.BlobType, restic.ID, []byte) (int, error)
	SaveBlobFn func(restic.BlobType, []byte, restic.ID) (restic.ID, error)

	LoadTreeFn func(restic.ID) (*restic.Tree, error)
	SaveTreeFn func(t *restic.Tree) (restic.ID, error)
}

// Backend is a stub method.
func (repo Repository) Backend() restic.Backend {
	return repo.BackendFn()
}

// Key is a stub method.
func (repo Repository) Key() *crypto.Key {
	return repo.KeyFn()
}

// SetIndex is a stub method.
func (repo Repository) SetIndex(idx restic.Index) error {
	return repo.SetIndexFn(idx)
}

// Index is a stub method.
func (repo Repository) Index() restic.Index {
	return repo.IndexFn()
}

// SaveFullIndex is a stub method.
func (repo Repository) SaveFullIndex() error {
	return repo.SaveFullIndexFn()
}

// SaveIndex is a stub method.
func (repo Repository) SaveIndex() error {
	return repo.SaveIndexFn()
}

// LoadIndex is a stub method.
func (repo Repository) LoadIndex() error {
	return repo.LoadIndexFn()
}

// Config is a stub method.
func (repo Repository) Config() restic.Config {
	return repo.ConfigFn()
}

// LookupBlobSize is a stub method.
func (repo Repository) LookupBlobSize(id restic.ID, t restic.BlobType) (uint, error) {
	return repo.LookupBlobSizeFn(id, t)
}

// List is a stub method.
func (repo Repository) List(t restic.FileType, done <-chan struct{}) <-chan restic.ID {
	return repo.ListFn(t, done)
}

// ListPack is a stub method.
func (repo Repository) ListPack(id restic.ID) ([]restic.Blob, int64, error) {
	return repo.ListPackFn(id)
}

// Flush is a stub method.
func (repo Repository) Flush() error {
	return repo.FlushFn()
}

// SaveUnpacked is a stub method.
func (repo Repository) SaveUnpacked(t restic.FileType, buf []byte) (restic.ID, error) {
	return repo.SaveUnpackedFn(t, buf)
}

// SaveJSONUnpacked is a stub method.
func (repo Repository) SaveJSONUnpacked(t restic.FileType, item interface{}) (restic.ID, error) {
	return repo.SaveJSONUnpackedFn(t, item)
}

// LoadJSONUnpacked is a stub method.
func (repo Repository) LoadJSONUnpacked(t restic.FileType, id restic.ID, item interface{}) error {
	return repo.LoadJSONUnpackedFn(t, id, item)
}

// LoadAndDecrypt is a stub method.
func (repo Repository) LoadAndDecrypt(t restic.FileType, id restic.ID) ([]byte, error) {
	return repo.LoadAndDecryptFn(t, id)
}

// LoadBlob is a stub method.
func (repo Repository) LoadBlob(t restic.BlobType, id restic.ID, buf []byte) (int, error) {
	return repo.LoadBlobFn(t, id, buf)
}

// SaveBlob is a stub method.
func (repo Repository) SaveBlob(t restic.BlobType, buf []byte, id restic.ID) (restic.ID, error) {
	return repo.SaveBlobFn(t, buf, id)
}

// LoadTree is a stub method.
func (repo Repository) LoadTree(id restic.ID) (*restic.Tree, error) {
	return repo.LoadTreeFn(id)
}

// SaveTree is a stub method.
func (repo Repository) SaveTree(t *restic.Tree) (restic.ID, error) {
	return repo.SaveTreeFn(t)
}
