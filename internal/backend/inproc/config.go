package inproc

import (
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

type Config struct {
	Connections uint `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
	Remote      string
	provider    ServiceProvider
}

var defaultConfig = Config{
	Connections: 5,
}

func init() {
	options.Register("inproc", Config{})
}

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return defaultConfig
}

// ParseConfig parses the string s and extracts the remote server URL.
func ParseConfig(s string) (interface{}, error) {
	if !strings.HasPrefix(s, "inproc:") {
		return nil, errors.New("invalid inproc backend specification")
	}

	items := strings.SplitN(s, ":", 3)
	if len(items) != 3 {
		return nil, errors.New("invalid URL")
	}

	provider, err := findServiceProvider(items[1])
	if err != nil {
		return nil, err
	}

	cfg := NewConfig()
	cfg.Remote = items[2]
	cfg.provider = provider
	return cfg, nil
}
