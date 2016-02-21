package gcs

import (
	"errors"
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
	return s3.NewConfig(gcsEndpoint, p, false)
}
