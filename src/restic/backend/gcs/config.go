package gcs

import (
	"errors"
	"path"
	"restic/backend/s3"
	"strings"
)

// The endpoint for all GCS operations.
const gcsEndpoint = "storage.googleapis.com"

const defaultPrefix = "restic"

// ParseConfig parses the string s and extracts the gcs config. The two
// supported configuration formats are gcs://bucketname/prefix and
// gcs:bucketname/prefix.
func ParseConfig(s string) (interface{}, error) {
	switch {
	case strings.HasPrefix(s, "gs://"):
		s = s[5:]
	case strings.HasPrefix(s, "gs:"):
		s = s[3:]
	default:
		return nil, errors.New(`gcs: config does not start with "gs"`)
	}
	p := strings.SplitN(s, "/", 2)
	var prefix string
	switch {
	case len(p) < 1:
		return nil, errors.New("gcs: invalid format: bucket name not found")
	case len(p) == 1 || p[1] == "":
		prefix = defaultPrefix
	default:
		prefix = path.Clean(p[1])
	}
	return s3.Config{
		Endpoint: gcsEndpoint,
		UseHTTP:  false,
		Bucket:   p[0],
		Prefix:   prefix,
	}, nil
}
