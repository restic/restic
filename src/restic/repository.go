package restic

import (
	"restic/backend"
	"restic/pack"
)

// Repository manages encrypted and packed data stored in a backend.
type Repository interface {
	LoadJSONUnpacked(backend.Type, backend.ID, interface{}) error
	SaveJSONUnpacked(backend.Type, interface{}) (backend.ID, error)

	Lister
}

// Lister combines lists packs in a repo and blobs in a pack.
type Lister interface {
	List(backend.Type, <-chan struct{}) <-chan backend.ID
	ListPack(backend.ID) ([]pack.Blob, int64, error)
}
