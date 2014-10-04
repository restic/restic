package backend

type Type string

const (
	Blob     Type = "blob"
	Key           = "key"
	Lock          = "lock"
	Snapshot      = "snapshot"
	Tree          = "tree"
)

const (
	BackendVersion = 1
)

type Server interface {
	Create(Type, []byte) (ID, error)
	Get(Type, ID) ([]byte, error)
	List(Type) (IDs, error)
	Test(Type, ID) (bool, error)
	Remove(Type, ID) error
	Version() uint

	Close() error

	Location() string
}
