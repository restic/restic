package s3

import (
	"errors"
	"strings"
)

// Config contains all configuration necessary to connect to an s3 compatible
// server.
type Config struct {
	Host          string
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
			Host:   data[0],
			Bucket: data[1],
		}

		return cfg, nil
	}

	data := strings.SplitN(s, ":", 3)
	if len(data) != 3 {
		return nil, errors.New("s3: invalid format")
	}

	if data[0] != "s3" {
		return nil, errors.New(`s3: config does not start with "s3"`)
	}

	cfg := Config{
		Host:   data[1],
		Bucket: data[2],
	}

	return cfg, nil
}
