package restic

import (
	"fmt"

	"github.com/restic/restic/internal/errors"
)

// FileType is the type of a file in the backend.
type FileType string

// These are the different data types a backend can store.
const (
	PackFile     FileType = "data" // use data, as packs are stored under /data in repo
	KeyFile      FileType = "key"
	LockFile     FileType = "lock"
	SnapshotFile FileType = "snapshot"
	IndexFile    FileType = "index"
	ConfigFile   FileType = "config"
)

// Handle is used to store and access data in a backend.
type Handle struct {
	Type              FileType
	ContainedBlobType BlobType
	Name              string
}

func (h Handle) String() string {
	name := h.Name
	if len(name) > 10 {
		name = name[:10]
	}
	return fmt.Sprintf("<%s/%s>", h.Type, name)
}

// Valid returns an error if h is not valid.
func (h Handle) Valid() error {
	if h.Type == "" {
		return errors.New("type is empty")
	}

	switch h.Type {
	case PackFile:
	case KeyFile:
	case LockFile:
	case SnapshotFile:
	case IndexFile:
	case ConfigFile:
	default:
		return errors.Errorf("invalid Type %q", h.Type)
	}

	if h.Type == ConfigFile {
		return nil
	}

	if h.Name == "" {
		return errors.New("invalid Name")
	}

	return nil
}
