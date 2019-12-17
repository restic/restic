package oss

import (
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
	"path"
	"strings"
)

// Config contains all configuration necessary to connect to an oss compatible
// server.
type Config struct {
	Endpoint        string
	AccessKeyID     string
	AccessKeySecret string
	Bucket          string
	Prefix          string

	Connections uint `option:"connections" help:"set a limit for the number of concurrent connections (default: 20)"`
}

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return Config{
		Connections: 5,
	}
}

func init() {
	options.Register("oss", Config{})
}

// ParseConfig parses the string s and extracts the oss config. The
// supported configuration formats are oss:host/bucketname/prefix.
func ParseConfig(s string) (interface{}, error) {
	if !strings.HasPrefix(s, "oss:") {
		return nil, errors.New("oss: invalid format")
	}

	// strip prefix "gs:"
	s = s[4:]

	// use the first entry of the path as the endpoint and the
	// remainder as bucket name and prefix
	path := strings.SplitN(s, "/", 3)
	return createConfig(path[0], path[1:], false)
}

func createConfig(endpoint string, p []string, useHTTP bool) (interface{}, error) {
	if len(p) < 1 {
		return nil, errors.New("oss: invalid format, host/region or bucket name not found")
	}

	var prefix string
	if len(p) > 1 && p[1] != "" {
		prefix = path.Clean(p[1])
	}

	cfg := NewConfig()
	cfg.Endpoint = endpoint
	cfg.Bucket = p[0]
	cfg.Prefix = prefix
	return cfg, nil
}
