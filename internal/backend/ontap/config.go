package ontap

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

func init() {
	options.Register(ProtocolScheme, Config{})
}

type Config struct {
	Prefix      string
	Bucket      *string
	Connections uint `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
	url         *url.URL
}

func (c *Config) GetAPIURL() string {
	return fmt.Sprintf("%s://%s", c.url.Scheme, c.url.Host)
}

// ParseConfig creates a new Config based on the given string.
//
// Supported configuration formats:
// - ontaps3:http://address/bucketname
// - ontaps3:address/bucketname
//
// NOTE: Will default to "https" if no HTTP protocol is specified
//
func ParseConfig(configStr string) (interface{}, error) {
	debug.Log("Config string: %s\n", configStr)

	if !strings.HasPrefix(configStr, ProtocolScheme+":") {
		return nil, errors.New(ProtocolScheme + ": invalid format")
	}
	configStr = configStr[len(ProtocolScheme)+1:]

	var ontapS3APIURL *url.URL
	var err error

	if strings.HasPrefix(configStr, "http://") || strings.HasPrefix(configStr, "https://") {
		ontapS3APIURL, err = url.Parse(configStr)
	} else {
		ontapS3APIURL, err = url.Parse("https://" + configStr)
	}

	if err != nil {
		return nil, errors.Wrap(err, "url.Parse")
	}

	if ontapS3APIURL.Path == "" {
		return nil, errors.New(ProtocolScheme + ": bucket name not found")
	}

	var prefix string
	if p := strings.SplitN(ontapS3APIURL.Path[1:], "/", 2); len(p) > 1 {
		prefix = p[1]
	}

	bucket := strings.Trim(ontapS3APIURL.Path, prefix)
	bucket = strings.Trim(bucket, "/")

	if bucket == "" {
		return nil, errors.New(ProtocolScheme + ": bucket name not found")
	}

	cfg := Config{
		Prefix:      path.Clean(prefix),
		Bucket:      &bucket,
		Connections: 5,
		url:         ontapS3APIURL,
	}

	return cfg, nil
}
