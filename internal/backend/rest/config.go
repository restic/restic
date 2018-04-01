package rest

import (
	"net/url"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to connect to a REST server.
type Config struct {
	URL         *url.URL
	Connections uint `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
}

func init() {
	options.Register("rest", Config{})
}

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return Config{
		Connections: 5,
	}
}

// ParseConfig parses the string s and extracts the REST server URL.
func ParseConfig(s string) (interface{}, error) {
	if !strings.HasPrefix(s, "rest:") {
		return nil, errors.New("invalid REST backend specification")
	}

	s = s[5:]
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}

	u, err := url.Parse(s)

	if err != nil {
		return nil, errors.Wrap(err, "url.Parse")
	}

	cfg := NewConfig()
	cfg.URL = u
	return cfg, nil
}
