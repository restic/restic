package s3

import (
	"errors"
	"net/url"
	"strings"
)

// Config contains all configuration necessary to connect to an s3 compatible
// server.
type Config struct {
	Endpoint      string
	UseHTTP       bool
	KeyID, Secret string
	Bucket        string
	Prefix        string
}

const defaultPrefix = "restic"

// ParseConfig parses the string s and extracts the s3 config. The two
// supported configuration formats are s3://host/bucketname/prefix and
// s3:host:bucketname/prefix. The host can also be a valid s3 region
// name. If no prefix is given the prefix "restic" will be used.
func ParseConfig(s string) (interface{}, error) {
	switch {
	case strings.HasPrefix(s, "s3:http"):
		// assume that a URL has been specified, parse it and
		// use the host as the endpoint and the path as the
		// bucket name and prefix
		url, err := url.Parse(s[3:])
		if err != nil {
			return nil, err
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

func createConfig(endpoint string, path []string, useHTTP bool) (interface{}, error) {
	var prefix string
	switch {
	case len(path) < 1:
		return nil, errors.New("s3: invalid format, host/region or bucket name not found")
	case len(path) == 1:
		prefix = defaultPrefix
	default:
		prefix = strings.TrimRight(path[1], "/")
	}
	return Config{
		Endpoint: endpoint,
		UseHTTP:  useHTTP,
		Bucket:   path[0],
		Prefix:   prefix,
	}, nil
}
