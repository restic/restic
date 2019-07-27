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
	if !strings.HasPrefix(s, "azure:") {
		return nil, errors.New("azure: invalid format")
	}

	// strip prefix "azure:"
	s = s[6:]

	// use the first entry of the path as the bucket name and the
	// remainder as prefix
	data := strings.SplitN(s, ":", 2)
	if len(data) < 2 {
		return nil, errors.New("azure: invalid format: bucket name or path not found")
	}
	container, path := data[0], path.Clean(data[1])
	path = strings.TrimPrefix(path, "/")
	cfg := NewConfig()
	cfg.Container = container
	cfg.Prefix = path
	return cfg, nil
}
