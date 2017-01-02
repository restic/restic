package b2

import (
	"path"
	"strings"

	"restic/errors"
)

// Config contains all configuration necessary to connect to an b2 compatible
// server.
type Config struct {
	AccountID string
	Key       string
	Bucket    string
	Prefix    string
}

const defaultPrefix = "restic"

// ParseConfig parses the string s and extracts the b2 config. The supported
// configuration format is b2:bucketname/prefix. If no prefix is given the
// prefix "restic" will be used.
func ParseConfig(s string) (interface{}, error) {
	switch {
	case strings.HasPrefix(s, "b2:"):
		s = s[3:]
	default:
		return nil, errors.New("b2: invalid format")
	}
	// use the remainder of the string as bucket name and prefix
	path := strings.SplitN(s, "/", 2)
	return createConfig(path)
}

func createConfig(p []string) (interface{}, error) {
	var prefix string
	switch {
	case len(p) < 1:
		return nil, errors.New("b2: invalid format, bucket name not found")
	case len(p) == 1 || p[1] == "":
		prefix = defaultPrefix
	default:
		prefix = path.Clean(p[1])
	}
	return Config{
		Bucket: p[0],
		Prefix: prefix,
	}, nil
}
