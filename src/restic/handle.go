package restic

import (
	"fmt"

	"github.com/pkg/errors"
)

// Handle is used to store and access data in a backend.
type Handle struct {
	FileType FileType
	Name     string
}

func (h Handle) String() string {
	name := h.Name
	if len(name) > 10 {
		name = name[:10]
	}
	return fmt.Sprintf("<%s/%s>", h.FileType, name)
}

// Valid returns an error if h is not valid.
func (h Handle) Valid() error {
	if h.FileType == "" {
		return errors.New("type is empty")
	}

	switch h.FileType {
	case DataFile:
	case KeyFile:
	case LockFile:
	case SnapshotFile:
	case IndexFile:
	case ConfigFile:
	default:
		return errors.Errorf("invalid Type %q", h.FileType)
	}

	if h.FileType == ConfigFile {
		return nil
	}

	if h.Name == "" {
		return errors.New("invalid Name")
	}

	return nil
}
