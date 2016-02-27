package gcs

import (
	"errors"
	"net/url"
	"restic/backend/s3"
	"strings"
)

// The endpoint for all GCS operations.
const gcsEndpoint = "storage.googleapis.com"

const defaultPrefix = "restic"

// ParseConfig parses the string s and extracts the s3 config. The two
// supported configuration formats are gs://bucketname/prefix and
// gs:bucketname/prefix.
//
// If no prefix is given the prefix "restic" will be used.
func ParseConfig(s string) (interface{}, error) {
	s = strings.TrimRight(s, "/") // remove trailing slashes
	switch {
	case strings.HasPrefix(s, "gs:http"):
		// assume that a URL has been specified, parse it and
		// use the host as the endpoint and the path as the
		// bucket name and prefix
		url, err := url.Parse(s[3:])
		if err != nil {
			return nil, err
		}

		if url.Path == "" {
			return nil, errors.New("gs: bucket name not found")
		}

		path := strings.SplitN(url.Path[1:], "/", 2)
		// create an s3 configuration using the local
		// compatible repository layout
		return s3.NewConfig(url.Host, path, url.Scheme == "http", true)
	case strings.HasPrefix(s, "gs://"):
		s = s[5:]
	case strings.HasPrefix(s, "gs:"):
		s = s[3:]
	default:
		return nil, errors.New(`gcs: config does not start with "gs"`)
	}
	// use the first entry of the path as bucket and the
	// remainder as prefix
	p := strings.SplitN(s, "/", 2)
	return s3.NewConfig(gcsEndpoint, p, false, true)
}
