package s3

import (
	"net/url"
	"path"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to connect to an s3 compatible
// server.
type Config struct {
	Endpoint      string
	UseHTTP       bool
	KeyID, Secret string
	Bucket        string
	Prefix        string
	Layout        string `option:"layout" help:"use this backend layout (default: auto-detect)"`
	StorageClass  string `option:"storage-class" help:"set S3 storage class (STANDARD, STANDARD_IA, ONEZONE_IA, INTELLIGENT_TIERING or REDUCED_REDUNDANCY)"`

	Connections uint `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
	MaxRetries  uint `option:"retries" help:"set the number of retries attempted"`
}

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return Config{
		Connections: 5,
	}
}

func init() {
	options.Register("s3", Config{})
}

// ParseConfig parses the string s and extracts the s3 config. The two
// supported configuration formats are s3://host/bucketname/prefix and
// s3:host/bucketname/prefix. The host can also be a valid s3 region
// name. If no prefix is given the prefix "restic" will be used.
func ParseConfig(s string) (interface{}, error) {
	switch {
	case strings.HasPrefix(s, "s3:http"):
		// assume that a URL has been specified, parse it and
		// use the host as the endpoint and the path as the
		// bucket name and prefix
		url, err := url.Parse(s[3:])
		if err != nil {
			return nil, errors.Wrap(err, "url.Parse")
		}

		if url.Path == "" {
			return nil, errors.New("s3: bucket name not found")
		}

		path := strings.SplitN(url.Path[1:], "/", 2)
		return createConfig(url.Host, path, url.Scheme == "http")
	case strings.HasPrefix(s, "s3://"):
		s = s[5:]
	case strings.HasPrefix(s, "s3:"):
		s = s[3:]
	default:
		return nil, errors.New("s3: invalid format")
	}
	// use the first entry of the path as the endpoint and the
	// remainder as bucket name and prefix
	path := strings.SplitN(s, "/", 3)
	return createConfig(path[0], path[1:], false)
}

func createConfig(endpoint string, p []string, useHTTP bool) (interface{}, error) {
	if len(p) < 1 {
		return nil, errors.New("s3: invalid format, host/region or bucket name not found")
	}

	var prefix string
	if len(p) > 1 && p[1] != "" {
		prefix = path.Clean(p[1])
	}

	cfg := NewConfig()
	cfg.Endpoint = endpoint
	cfg.UseHTTP = useHTTP
	cfg.Bucket = p[0]
	cfg.Prefix = prefix
	return cfg, nil
}
