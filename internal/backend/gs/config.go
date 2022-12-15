package gs

import (
	"path"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to connect to a Google Cloud Storage
// bucket. We use Google's default application credentials to acquire an access token, so
// we don't require that calling code supply any authentication material here.
type Config struct {
	ProjectID string
	Bucket    string
	Prefix    string

	Connections uint `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
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
	if !strings.HasPrefix(s, "gs:") {
		return nil, errors.New("gs: invalid format")
	}

	// strip prefix "gs:"
	s = s[3:]

	// use the first entry of the path as the bucket name and the
	// remainder as prefix
	bucket, prefix, colon := strings.Cut(s, ":")
	if !colon {
		return nil, errors.New("gs: invalid format: bucket name or path not found")
	}

	prefix = strings.TrimPrefix(path.Clean(prefix), "/")

	cfg := NewConfig()
	cfg.Bucket = bucket
	cfg.Prefix = prefix
	return cfg, nil
}
