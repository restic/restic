package b2

import (
	"path"
	"regexp"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
)

// Config contains all configuration necessary to connect to an b2 compatible
// server.
type Config struct {
	AccountID string
	Key       options.SecretString
	Bucket    string
	Prefix    string

	Connections uint `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
}

// NewConfig returns a new config with default options applied.
func NewConfig() Config {
	return Config{
		Connections: 5,
	}
}

func init() {
	options.Register("b2", Config{})
}

var bucketName = regexp.MustCompile("^[a-zA-Z0-9-]+$")

// checkBucketName tests the bucket name against the rules at
// https://help.backblaze.com/hc/en-us/articles/217666908-What-you-need-to-know-about-B2-Bucket-names
func checkBucketName(name string) error {
	if name == "" {
		return errors.New("bucket name is empty")
	}

	if len(name) < 6 {
		return errors.New("bucket name is too short")
	}

	if len(name) > 50 {
		return errors.New("bucket name is too long")
	}

	if !bucketName.MatchString(name) {
		return errors.New("bucket name contains invalid characters, allowed are: a-z, 0-9, dash (-)")
	}

	return nil
}

// ParseConfig parses the string s and extracts the b2 config. The supported
// configuration format is b2:bucketname/prefix. If no prefix is given the
// prefix "restic" will be used.
func ParseConfig(s string) (interface{}, error) {
	if !strings.HasPrefix(s, "b2:") {
		return nil, errors.New("invalid format, want: b2:bucket-name[:path]")
	}

	s = s[3:]
	data := strings.SplitN(s, ":", 2)
	if len(data) == 0 || len(data[0]) == 0 {
		return nil, errors.New("bucket name not found")
	}

	cfg := NewConfig()
	cfg.Bucket = data[0]

	if err := checkBucketName(cfg.Bucket); err != nil {
		return nil, err
	}

	if len(data) == 2 {
		p := data[1]
		if len(p) > 0 {
			p = path.Clean(p)
		}

		if len(p) > 0 && path.IsAbs(p) {
			p = p[1:]
		}

		cfg.Prefix = p
	}

	return cfg, nil
}
