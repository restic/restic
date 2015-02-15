package backend

import (
	"errors"
	"io"
)

type Type string

const (
	Data     Type = "data"
	Key           = "key"
	Lock          = "lock"
	Snapshot      = "snapshot"
	Tree          = "tree"
)

const (
	BackendVersion = 1
)

var (
	ErrAlreadyPresent = errors.New("blob is already present in backend")
)

type Blob interface {
	io.WriteCloser
	ID() (ID, error)
}

type Lister interface {
	List(Type) (IDs, error)
}

type Getter interface {
	Get(Type, ID) ([]byte, error)
	GetReader(Type, ID) (io.ReadCloser, error)
}

type Creater interface {
	Create(Type, []byte) (ID, error)
	CreateFrom(Type, io.Reader) (ID, error)
	CreateBlob(Type) (Blob, error)
}

type Tester interface {
	Test(Type, ID) (bool, error)
}

type Remover interface {
	Remove(Type, ID) error
}

type Closer interface {
	Close() error
}

type Deleter interface {
	Delete() error
}

type Locationer interface {
	Location() string
}

type Backend interface {
	Lister
	Getter
	Creater
	Tester
	Remover
	Closer
}
