package gs

import (
	"path"
	"strings"

	"restic/errors"
	"restic/options"
)

// Config contains all configuration necessary to connect to an gcs compatible
// server.
type Config struct {
	ProjectID   string
	JSONKeyPath string
	Bucket      string
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
	options.Register("gs", Config{})
}

// ParseConfig parses the string s and extracts the gcs config. The
// supported configuration format is gs:bucketName:/[prefix].
func ParseConfig(s string) (interface{}, error) {
	if strings.HasPrefix(s, "gs:") {
		s = s[3:]
	} else {
		return nil, errors.New("gs: invalid format")
	}
	// use the first entry of the path as the bucket name and the
	// remainder as prefix
	path := strings.SplitN(s, ":/", 2)
	return createConfig(path)
}

func createConfig(p []string) (interface{}, error) {
	if len(p) < 2 {
		return nil, errors.New("gs: invalid format, bucket name not found")
	}
	cfg := NewConfig()
	cfg.Bucket = p[0]
	if p[1] != "" {
		cfg.Prefix = path.Clean(p[1])
	}
	return cfg, nil
}
