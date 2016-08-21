package backend

import (
	"fmt"

	"github.com/pkg/errors"
)

// Handle is used to store and access data in a backend.
type Handle struct {
	Type Type
	Name string
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
	case Data:
	case Key:
	case Lock:
	case Snapshot:
	case Index:
	case Config:
	default:
		return fmt.Errorf("invalid Type %q", h.Type)
	}

	if h.Type == Config {
		return nil
	}

	if h.Name == "" {
		return errors.New("invalid Name")
	}

	return nil
}
