package local

import (
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config holds all information needed to open a local repository.
type Config struct {
	Path   string
	Layout string `option:"layout" help:"use this backend directory layout (default: auto-detect) (deprecated)"`

	Connections uint `option:"connections" help:"set a limit for the number of concurrent operations (default: 2)"`
}

// NewConfig returns a new config with default options applied.
func NewConfig() Config {
	return Config{
		Connections: 2,
	}
}

func init() {
	options.Register("local", Config{})
}

// ParseConfig parses a local backend config.
func ParseConfig(s string) (*Config, error) {
	if !strings.HasPrefix(s, "local:") {
		return nil, errors.New(`invalid format, prefix "local" not found`)
	}

	cfg := NewConfig()
	cfg.Path = s[6:]
	return &cfg, nil
}
