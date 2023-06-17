package s3

import (
	"strings"
	"testing"

	"github.com/restic/restic/internal/backend/test"
)

var configTests = []test.ConfigTestData[Config]{
	{S: "s3://eu-central-1/bucketname", Cfg: Config{
		Endpoint:    "eu-central-1",
		Bucket:      "bucketname",
		Prefix:      "",
		Connections: 5,
	}},
	{S: "s3://eu-central-1/bucketname/", Cfg: Config{
		Endpoint:    "eu-central-1",
		Bucket:      "bucketname",
		Prefix:      "",
		Connections: 5,
	}},
	{S: "s3://eu-central-1/bucketname/prefix/directory", Cfg: Config{
		Endpoint:    "eu-central-1",
		Bucket:      "bucketname",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
	{S: "s3://eu-central-1/bucketname/prefix/directory/", Cfg: Config{
		Endpoint:    "eu-central-1",
		Bucket:      "bucketname",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
	{S: "s3:eu-central-1/foobar", Cfg: Config{
		Endpoint:    "eu-central-1",
		Bucket:      "foobar",
		Prefix:      "",
		Connections: 5,
	}},
	{S: "s3:eu-central-1/foobar/", Cfg: Config{
		Endpoint:    "eu-central-1",
		Bucket:      "foobar",
		Prefix:      "",
		Connections: 5,
	}},
	{S: "s3:eu-central-1/foobar/prefix/directory", Cfg: Config{
		Endpoint:    "eu-central-1",
		Bucket:      "foobar",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
	{S: "s3:eu-central-1/foobar/prefix/directory/", Cfg: Config{
		Endpoint:    "eu-central-1",
		Bucket:      "foobar",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
	{S: "s3:hostname.foo/foobar", Cfg: Config{
		Endpoint:    "hostname.foo",
		Bucket:      "foobar",
		Prefix:      "",
		Connections: 5,
	}},
	{S: "s3:hostname.foo/foobar/prefix/directory", Cfg: Config{
		Endpoint:    "hostname.foo",
		Bucket:      "foobar",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
	{S: "s3:https://hostname/foobar", Cfg: Config{
		Endpoint:    "hostname",
		Bucket:      "foobar",
		Prefix:      "",
		Connections: 5,
	}},
	{S: "s3:https://hostname:9999/foobar", Cfg: Config{
		Endpoint:    "hostname:9999",
		Bucket:      "foobar",
		Prefix:      "",
		Connections: 5,
	}},
	{S: "s3:https://hostname:9999/foobar/", Cfg: Config{
		Endpoint:    "hostname:9999",
		Bucket:      "foobar",
		Prefix:      "",
		Connections: 5,
	}},
	{S: "s3:http://hostname:9999/foobar", Cfg: Config{
		Endpoint:    "hostname:9999",
		Bucket:      "foobar",
		Prefix:      "",
		UseHTTP:     true,
		Connections: 5,
	}},
	{S: "s3:http://hostname:9999/foobar/", Cfg: Config{
		Endpoint:    "hostname:9999",
		Bucket:      "foobar",
		Prefix:      "",
		UseHTTP:     true,
		Connections: 5,
	}},
	{S: "s3:http://hostname:9999/bucket/prefix/directory", Cfg: Config{
		Endpoint:    "hostname:9999",
		Bucket:      "bucket",
		Prefix:      "prefix/directory",
		UseHTTP:     true,
		Connections: 5,
	}},
	{S: "s3:http://hostname:9999/bucket/prefix/directory/", Cfg: Config{
		Endpoint:    "hostname:9999",
		Bucket:      "bucket",
		Prefix:      "prefix/directory",
		UseHTTP:     true,
		Connections: 5,
	}},
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
