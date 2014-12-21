package backend

import "errors"

type Type string

const (
	Data     Type = "data"
	Key           = "key"
	Lock          = "lock"
	Snapshot      = "snapshot"
	Tree          = "tree"
	Map           = "map"
)

const (
	BackendVersion = 1
)

var (
	ErrAlreadyPresent = errors.New("blob is already present in backend")
)

type lister interface {
	List(Type) (IDs, error)
}

type getter interface {
	Get(Type, ID) ([]byte, error)
}

type creater interface {
	Create(Type, []byte) (ID, error)
}

type tester interface {
	Test(Type, ID) (bool, error)
}

type remover interface {
	Remove(Type, ID) error
}

type closer interface {
	Close() error
}

type deleter interface {
	Delete() error
}

type locationer interface {
	Location() string
}

type backend interface {
	lister
	getter
	creater
	tester
	remover
	closer
}
