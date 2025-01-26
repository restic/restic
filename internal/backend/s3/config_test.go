package s3

import (
	"strings"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend/test"
)

func newTestConfig(cfg Config) Config {
	if cfg.Connections == 0 {
		cfg.Connections = 5
	}
	if cfg.RestoreDays == 0 {
		cfg.RestoreDays = 7
	}
	if cfg.RestoreTimeout == 0 {
		cfg.RestoreTimeout = 24 * time.Hour
	}
	if cfg.RestoreTier == "" {
		cfg.RestoreTier = "Standard"
	}
	return cfg
}

var configTests = []test.ConfigTestData[Config]{
	{S: "s3://eu-central-1/bucketname", Cfg: newTestConfig(Config{
		Endpoint: "eu-central-1",
		Bucket:   "bucketname",
		Prefix:   "",
	})},
	{S: "s3://eu-central-1/bucketname/", Cfg: newTestConfig(Config{
		Endpoint: "eu-central-1",
		Bucket:   "bucketname",
		Prefix:   "",
	})},
	{S: "s3://eu-central-1/bucketname/prefix/directory", Cfg: newTestConfig(Config{
		Endpoint: "eu-central-1",
		Bucket:   "bucketname",
		Prefix:   "prefix/directory",
	})},
	{S: "s3://eu-central-1/bucketname/prefix/directory/", Cfg: newTestConfig(Config{
		Endpoint: "eu-central-1",
		Bucket:   "bucketname",
		Prefix:   "prefix/directory",
	})},
	{S: "s3:eu-central-1/foobar", Cfg: newTestConfig(Config{
		Endpoint: "eu-central-1",
		Bucket:   "foobar",
		Prefix:   "",
	})},
	{S: "s3:eu-central-1/foobar/", Cfg: newTestConfig(Config{
		Endpoint: "eu-central-1",
		Bucket:   "foobar",
		Prefix:   "",
	})},
	{S: "s3:eu-central-1/foobar/prefix/directory", Cfg: newTestConfig(Config{
		Endpoint: "eu-central-1",
		Bucket:   "foobar",
		Prefix:   "prefix/directory",
	})},
	{S: "s3:eu-central-1/foobar/prefix/directory/", Cfg: newTestConfig(Config{
		Endpoint: "eu-central-1",
		Bucket:   "foobar",
		Prefix:   "prefix/directory",
	})},
	{S: "s3:hostname.foo/foobar", Cfg: newTestConfig(Config{
		Endpoint: "hostname.foo",
		Bucket:   "foobar",
		Prefix:   "",
	})},
	{S: "s3:hostname.foo/foobar/prefix/directory", Cfg: newTestConfig(Config{
		Endpoint: "hostname.foo",
		Bucket:   "foobar",
		Prefix:   "prefix/directory",
	})},
	{S: "s3:https://hostname/foobar", Cfg: newTestConfig(Config{
		Endpoint: "hostname",
		Bucket:   "foobar",
		Prefix:   "",
	})},
	{S: "s3:https://hostname:9999/foobar", Cfg: newTestConfig(Config{
		Endpoint: "hostname:9999",
		Bucket:   "foobar",
		Prefix:   "",
	})},
	{S: "s3:https://hostname:9999/foobar/", Cfg: newTestConfig(Config{
		Endpoint: "hostname:9999",
		Bucket:   "foobar",
		Prefix:   "",
	})},
	{S: "s3:http://hostname:9999/foobar", Cfg: newTestConfig(Config{
		Endpoint: "hostname:9999",
		Bucket:   "foobar",
		Prefix:   "",
		UseHTTP:  true,
	})},
	{S: "s3:http://hostname:9999/foobar/", Cfg: newTestConfig(Config{
		Endpoint: "hostname:9999",
		Bucket:   "foobar",
		Prefix:   "",
		UseHTTP:  true,
	})},
	{S: "s3:http://hostname:9999/bucket/prefix/directory", Cfg: newTestConfig(Config{
		Endpoint: "hostname:9999",
		Bucket:   "bucket",
		Prefix:   "prefix/directory",
		UseHTTP:  true,
	})},
	{S: "s3:http://hostname:9999/bucket/prefix/directory/", Cfg: newTestConfig(Config{
		Endpoint: "hostname:9999",
		Bucket:   "bucket",
		Prefix:   "prefix/directory",
		UseHTTP:  true,
	})},
}

func TestParseConfig(t *testing.T) {
	test.ParseConfigTester(t, ParseConfig, configTests)
}

func TestParseError(t *testing.T) {
	const prefix = "s3: invalid format,"

	for _, s := range []string{"", "/", "//", "/bucket/prefix"} {
		_, err := ParseConfig("s3://" + s)
		if err == nil || !strings.HasPrefix(err.Error(), prefix) {
			t.Errorf("expected %q, got %q", prefix, err)
		}
	}
}
