package s3

import (
	"net/url"
	"path"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to connect to an s3 compatible
// server.
type Config struct {
	Endpoint     string
	UseHTTP      bool
	KeyID        string
	Secret       options.SecretString
	Bucket       string
	Prefix       string
	Layout       string `option:"layout" help:"use this backend layout (default: auto-detect)"`
	StorageClass string `option:"storage-class" help:"set S3 storage class (STANDARD, STANDARD_IA, ONEZONE_IA, INTELLIGENT_TIERING or REDUCED_REDUNDANCY)"`

	Connections   uint   `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
	MaxRetries    uint   `option:"retries" help:"set the number of retries attempted"`
	Region        string `option:"region" help:"set region"`
	BucketLookup  string `option:"bucket-lookup" help:"bucket lookup style: 'auto', 'dns', or 'path'"`
	ListObjectsV1 bool   `option:"list-objects-v1" help:"use deprecated V1 api for ListObjects calls"`
}

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return Config{
		Connections:   5,
		ListObjectsV1: false,
	}
}

func init() {
	options.Register("s3", Config{})
}

// ParseConfig parses the string s and extracts the s3 config. The two
// supported configuration formats are s3://host/bucketname/prefix and
// s3:host/bucketname/prefix. The host can also be a valid s3 region
// name. If no prefix is given the prefix "restic" will be used.
func ParseConfig(s string) (interface{}, error) {
	switch {
	case strings.HasPrefix(s, "s3:http"):
		// assume that a URL has been specified, parse it and
		// use the host as the endpoint and the path as the
		// bucket name and prefix
		url, err := url.Parse(s[3:])
		if err != nil {
			return nil, errors.WithStack(err)
		}

		if url.Path == "" {
			return nil, errors.New("s3: bucket name not found")
		}

		bucket, path, _ := strings.Cut(url.Path[1:], "/")
		return createConfig(url.Host, bucket, path, url.Scheme == "http")
	case strings.HasPrefix(s, "s3://"):
		s = s[5:]
	case strings.HasPrefix(s, "s3:"):
		s = s[3:]
	default:
		return nil, errors.New("s3: invalid format")
	}
	// use the first entry of the path as the endpoint and the
	// remainder as bucket name and prefix
	endpoint, rest, _ := strings.Cut(s, "/")
	bucket, prefix, _ := strings.Cut(rest, "/")
	return createConfig(endpoint, bucket, prefix, false)
}

func createConfig(endpoint, bucket, prefix string, useHTTP bool) (interface{}, error) {
	if endpoint == "" {
		return nil, errors.New("s3: invalid format, host/region or bucket name not found")
	}

	if prefix != "" {
		prefix = path.Clean(prefix)
	}

	cfg := NewConfig()
	cfg.Endpoint = endpoint
	cfg.UseHTTP = useHTTP
	cfg.Bucket = bucket
	cfg.Prefix = prefix
	return cfg, nil
}
