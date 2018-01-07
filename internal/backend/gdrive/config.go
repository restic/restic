package gdrive

import (
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to connect to a Google Drive
type Config struct {
	JSONKeyPath string

	Prefix string

	Connections uint `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
	// Timeout     time.Duration `option:"timeout" help:"set remote request timeout (default: 5 minutes)"`
}

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return Config{
		Connections: 5,
		// Timeout:     5 * time.Minute,
	}
}

func init() {
	options.Register("gdrive", Config{})
}

// ParseConfig parses the string s and extracts the gcs config. The
// supported configuration format is gdrive:prefix.
func ParseConfig(s string) (interface{}, error) {
	data := strings.SplitN(s, ":", 2)
	if len(data) != 2 {
		return nil, errors.New("invalid URL, expected: gdrive:prefix")
	}

	scheme, prefix := data[0], data[1]

	if scheme != "gdrive" {
		return nil, errors.Errorf("unexpected schema: %s", scheme)
	}

	if len(prefix) == 0 {
		return nil, errors.Errorf("prefix is empty")
	}

	cfg := NewConfig()
	cfg.Prefix = prefix
	return cfg, nil
}
