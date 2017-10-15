package oss

import (
	"os"
	"strings"

	"github.com/pkg/errors"
)

// Config contains all configuration necessary to connect to a REST server.
type Config struct {
	Host      string
	AccessID  string
	AccessKey string
	Bucket    string
	Prefix    string

	Connections uint `option:"connections" help:"set a limit for the number of concurrent connections (default: 5)"`
}

// NewConfig returns a new Config with the default values filled in.
func NewConfig() Config {
	return Config{
		Connections: 5,
	}
}

// ParseConfig parses the string s and extracts the OSS config
func ParseConfig(s string) (interface{}, error) {
	data := strings.SplitN(s, ":", 6)
	if len(data) != 6 {
		return nil, errors.New("invalid URL, excepted: oss:accessid:accesskey:host:bucket:prefix")
	}
	schema, accessid, accesskey, host, bucket, prefix :=
		data[0], data[1], data[2], data[3], data[4], data[5]

	if schema != "oss" {
		return nil, errors.Errorf("unexpected prefix: %s", schema)
	}

	if host == "" && os.Getenv("RESTIC_OSS_HOST") != "" {
		host = os.Getenv("RESTIC_OSS_HOST")
	}
	if accessid == "" && os.Getenv("RESTIC_OSS_ID") != "" {
		accessid = os.Getenv("RESTIC_OSS_ID")
	}
	if accesskey == "" && os.Getenv("RESTIC_OSS_KEY") != "" {
		accesskey = os.Getenv("RESTIC_OSS_KEY")
	}
	if bucket == "" && os.Getenv("RESTIC_OSS_BUCKET") != "" {
		bucket = os.Getenv("RESTIC_OSS_BUCKET")
	}
	if prefix == "" && os.Getenv("RESTIC_OSS_PREFIX") != "" {
		prefix = os.Getenv("RESTIC_OSS_PREFIX")
	}

	cfg := NewConfig()
	cfg.Host = host
	cfg.AccessID = accessid
	cfg.AccessKey = accesskey
	cfg.Bucket = bucket
	cfg.Prefix = prefix

	return cfg, nil
}
