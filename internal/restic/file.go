package restic

import (
	"fmt"

	"github.com/restic/restic/internal/errors"
)

// FileType is the type of a file in the backend.
type FileType uint8

// These are the different data types a backend can store.
const (
	PackFile FileType = 1 + iota
	KeyFile
	LockFile
	SnapshotFile
	IndexFile
	ConfigFile
)

func (t FileType) String() string {
	s := "invalid"
	switch t {
	case PackFile:
		// Spelled "data" instead of "pack" for historical reasons.
		s = "data"
	case KeyFile:
		s = "key"
	case LockFile:
		s = "lock"
	case SnapshotFile:
		s = "snapshot"
	case IndexFile:
		s = "index"
	case ConfigFile:
		s = "config"
	}
	return s
}

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
	switch h.Type {
	case PackFile:
	case KeyFile:
	case LockFile:
	case SnapshotFile:
	case IndexFile:
	case ConfigFile:
	default:
		return errors.Errorf("invalid Type %d", h.Type)
	}

	if h.Type == ConfigFile {
		return nil
	}

	if h.Name == "" {
		return errors.New("invalid Name")
	}

	return nil
}
