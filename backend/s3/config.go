package s3

import (
	"errors"
	"net/url"
	"strings"
)

// Config contains all configuration necessary to connect to an s3 compatible
// server.
type Config struct {
	Region        string
	URL           string
	KeyID, Secret string
	Bucket        string
}

// ParseConfig parses the string s and extracts the s3 config. The two
// supported configuration formats are s3://host/bucketname and
// s3:host:bucketname. The host can also be a valid s3 region name.
func ParseConfig(s string) (interface{}, error) {
	if strings.HasPrefix(s, "s3://") {
		s = s[5:]

		data := strings.SplitN(s, "/", 2)
		if len(data) != 2 {
			return nil, errors.New("s3: invalid format, host/region or bucket name not found")
		}

		cfg := Config{
			Region: data[0],
			Bucket: data[1],
		}

		return cfg, nil
	}

	data := strings.SplitN(s, ":", 2)
	if len(data) != 2 {
		return nil, errors.New("s3: invalid format")
	}

	if data[0] != "s3" {
		return nil, errors.New(`s3: config does not start with "s3"`)
	}

	s = data[1]

	cfg := Config{}
	rest := strings.Split(s, "/")
	if len(rest) < 2 {
		return nil, errors.New("s3: region or bucket not found")
	}

	if len(rest) == 2 {
		// assume that just a region name and a bucket has been specified, in
		// the format region/bucket
		cfg.Region = rest[0]
		cfg.Bucket = rest[1]
	} else {
		// assume that a URL has been specified, parse it and use the path as
		// the bucket name.
		url, err := url.Parse(s)
		if err != nil {
			return nil, err
		}

		if url.Path == "" {
			return nil, errors.New("s3: bucket name not found")
		}

		cfg.Bucket = url.Path[1:]
		url.Path = ""

		cfg.URL = url.String()
	}

	return cfg, nil
}
