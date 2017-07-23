package local

import (
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config holds all information needed to open a local repository.
type Config struct {
	Path   string
	Layout string `option:"layout" help:"use this backend directory layout (default: auto-detect)"`
}

func init() {
	options.Register("local", Config{})
}

// ParseConfig parses a local backend config.
func ParseConfig(cfg string) (interface{}, error) {
	if !strings.HasPrefix(cfg, "local:") {
		return nil, errors.New(`invalid format, prefix "local" not found`)
	}

	return Config{Path: cfg[6:]}, nil
}
