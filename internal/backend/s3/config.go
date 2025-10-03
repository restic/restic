package s3

import (
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to connect to an s3 compatible
// server.
type Config struct {
	Endpoint     string
	UseHTTP      bool
	Bucket       string
	Prefix       string
	Layout       string `option:"layout" help:"use this backend layout (default: auto-detect) (deprecated)"`
	StorageClass string `option:"storage-class" help:"set S3 storage class (STANDARD, STANDARD_IA, ONEZONE_IA, INTELLIGENT_TIERING or REDUCED_REDUNDANCY)"`

	EnableRestore  bool          `option:"enable-restore" help:"restore objects from GLACIER or DEEP_ARCHIVE storage classes (default: false, requires \"s3-restore\" feature flag)"`
	RestoreDays    int           `option:"restore-days" help:"lifetime in days of restored object (default: 7)"`
	RestoreTimeout time.Duration `option:"restore-timeout" help:"maximum time to wait for objects transition (default: 24h)"`
	RestoreTier    string        `option:"restore-tier" help:"Retrieval tier at which the restore will be processed. (Standard, Bulk or Expedited) (default: Standard)"`

	Connections         uint   `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
	MaxRetries          uint   `option:"retries" help:"set the number of retries attempted"`
	Region              string `option:"region" help:"set region"`
	BucketLookup        string `option:"bucket-lookup" help:"bucket lookup style: 'auto', 'dns', or 'path'"`
	ListObjectsV1       bool   `option:"list-objects-v1" help:"use deprecated V1 api for ListObjects calls"`
	UnsafeAnonymousAuth bool   `option:"unsafe-anonymous-auth" help:"use anonymous authentication"`

	// For testing only
	KeyID  string
	Secret options.SecretString
}

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return Config{
		Connections:    5,
		ListObjectsV1:  false,
		EnableRestore:  false,
		RestoreDays:    7,
		RestoreTimeout: 24 * time.Hour,
		RestoreTier:    "Standard",
	}
}

func init() {
	options.Register("s3", Config{})
}

// ParseConfig parses the string s and extracts the s3 config. The two
// supported configuration formats are s3://host/bucketname/prefix and
// s3:host/bucketname/prefix. The host can also be a valid s3 region
// name. If no prefix is given the prefix "restic" will be used.
func ParseConfig(s string) (*Config, error) {
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

func createConfig(endpoint, bucket, prefix string, useHTTP bool) (*Config, error) {
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
	return &cfg, nil
}

var _ backend.ApplyEnvironmenter = &Config{}

// ApplyEnvironment saves values from the environment to the config.
func (cfg *Config) ApplyEnvironment(prefix string) {
	if cfg.Region == "" {
		cfg.Region = os.Getenv(prefix + "AWS_DEFAULT_REGION")
	}
}
