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
	Prefix	      string
}

const Scheme = "s3"

const defaultPrefix = "restic"

// ParseConfig parses the string s and extracts the s3 config. The two
// supported configuration formats are s3://host/bucketname/prefix and
// s3:host:bucketname/prefix. The host can also be a valid s3 region
// name. If no prefix is given the prefix "restic" will be used.
func ParseConfig(s string) (interface{}, error) {
     	var path []string
	cfg:= Config {};
	if strings.HasPrefix(s, "s3://") {
		s = s[5:]
	       	path = strings.SplitN(s, "/", 3)
		cfg.Endpoint = path[0]
		path = path[1:]
        } else if strings.HasPrefix(s, "s3:http") {
	       s = s[3:]
		// assume that a URL has been specified, parse it and
		// use the host as the endpoint and the path as the
		// bucket name and prefix
		url, err := url.Parse(s)
		if err != nil {
			return nil, err
		}

		if url.Path == "" {
			return nil, errors.New("s3: bucket name not found")
		}

		cfg.Endpoint = url.Host
		if url.Scheme == "http" {
			cfg.UseHTTP = true
		}
		path = strings.SplitN(url.Path[1:], "/", 2)
	} else if strings.HasPrefix(s, "s3:") {
	       s = s[3:]
	       path = strings.SplitN(s, "/",3)
	       cfg.Endpoint = path[0]
	       path = path[1:]
	} else {
		return nil, errors.New("s3: invalid format")
	}
	if len(path) < 1 {
		return nil, errors.New("s3: invalid format, host/region or bucket name not found")
	}
	cfg.Bucket = path[0];
	if len(path) > 1 {
	   cfg.Prefix = path[1]
	} else {
	   cfg.Prefix = defaultPrefix
        }

	return cfg, nil
}

