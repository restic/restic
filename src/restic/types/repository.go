package types

import (
	"restic"
	"restic/backend"
	"restic/pack"
)

// Repository manages encrypted and packed data stored in a backend.
type Repository interface {
	LoadJSONUnpacked(restic.FileType, backend.ID, interface{}) error
	SaveJSONUnpacked(restic.FileType, interface{}) (backend.ID, error)

	Lister
}

// Lister combines lists packs in a repo and blobs in a pack.
type Lister interface {
	List(restic.FileType, <-chan struct{}) <-chan backend.ID
	ListPack(backend.ID) ([]pack.Blob, int64, error)
}
