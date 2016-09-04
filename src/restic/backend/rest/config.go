package rest

import (
	"net/url"
	"strings"

	"restic/errors"
)

// Config contains all configuration necessary to connect to a REST server.
type Config struct {
	URL *url.URL
}

// ParseConfig parses the string s and extracts the REST server URL.
func ParseConfig(s string) (interface{}, error) {
	if !strings.HasPrefix(s, "rest:") {
		return nil, errors.New("invalid REST backend specification")
	}

	s = s[5:]
	u, err := url.Parse(s)

	if err != nil {
		return nil, errors.Wrap(err, "url.Parse")
	}

	cfg := Config{URL: u}
	return cfg, nil
}
