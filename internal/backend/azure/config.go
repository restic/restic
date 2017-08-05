package azure

import (
	"path"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to connect to an azure compatible
// server.
type Config struct {
	AccountName string
	AccountKey  string
	Container   string
	Prefix      string

	Connections uint `option:"connections" help:"set a limit for the number of concurrent connections (default: 20)"`
}

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return Config{
		Connections: 5,
	}
}

func init() {
	options.Register("azure", Config{})
}

// ParseConfig parses the string s and extracts the azure config. The
// configuration format is azure:containerName:/[prefix].
func ParseConfig(s string) (interface{}, error) {
	if strings.HasPrefix(s, "azure:") {
		s = s[6:]
	} else {
		return nil, errors.New("azure: invalid format")
	}
	// use the first entry of the path as the container name and the
	// remainder as prefix
	path := strings.SplitN(s, ":/", 2)
	return createConfig(path)
}

func createConfig(p []string) (interface{}, error) {
	if len(p) < 2 {
		return nil, errors.New("azure: invalid format, container name not found")
	}
	cfg := NewConfig()
	cfg.Container = p[0]
	if p[1] != "" {
		cfg.Prefix = path.Clean(p[1])
	}
	return cfg, nil
}
