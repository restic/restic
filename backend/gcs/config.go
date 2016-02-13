package gcs

import (
	"errors"
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

const Scheme = "gs"
const defaultPrefix = "restic"
const gcsEndpoint = "storage.googleapis.com"

// ParseConfig parses the string s and extracts the gcs config. The two
// supported configuration formats are gcs://bucketname/prefix and
// gcs:bucketname/prefix.
func ParseConfig(s string) (interface {}, error) {
	if strings.HasPrefix(s, "gs://") {
		s = s[5:]
	} else if strings.HasPrefix(s, "gs:") {
		s = s[3:]
	} else {
		return nil, errors.New(`gcs: config does not start with "gcs"`)
	}

	// be dfensive against wron user input and trim trailing slashes
	data := strings.SplitN(strings.TrimRight(s, "/"), "/", 2)
	if len(data) < 1 {
		return nil, errors.New("gcs: invalid format, bucket name not found")
	}
	prefix := defaultPrefix

	if len(data) > 1 {
	   prefix = data[1]
	}
	cfg := Config{
	    Endpoint: gcsEndpoint,
	    Bucket:   data[0],
	    Prefix:   prefix,
	}
	return cfg, nil
}
