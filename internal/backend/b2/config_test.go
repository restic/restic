package b2

import (
	"testing"

	"github.com/restic/restic/internal/backend/test"
)

var configTests = []test.ConfigTestData[Config]{
	{S: "b2:bucketname", Cfg: Config{
		Bucket:      "bucketname",
		Prefix:      "",
		Connections: 5,
	}},
	{S: "b2:bucketname:", Cfg: Config{
		Bucket:      "bucketname",
		Prefix:      "",
		Connections: 5,
	}},
	{S: "b2:bucketname:/prefix/directory", Cfg: Config{
		Bucket:      "bucketname",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
	{S: "b2:foobar", Cfg: Config{
		Bucket:      "foobar",
		Prefix:      "",
		Connections: 5,
	}},
	{S: "b2:foobar:", Cfg: Config{
		Bucket:      "foobar",
		Prefix:      "",
		Connections: 5,
	}},
	{S: "b2:foobar:/", Cfg: Config{
		Bucket:      "foobar",
		Prefix:      "",
		Connections: 5,
	}},
}

func TestParseConfig(t *testing.T) {
	test.ParseConfigTester(t, ParseConfig, configTests)
}

var invalidConfigTests = []struct {
	s   string
	err string
}{
	{
		"b2",
		"invalid format, want: b2:bucket-name[:path]",
	},
	{
		"b2:",
		"bucket name not found",
	},
	{
		"b2:bucket_name",
		"bucket name contains invalid characters, allowed are: a-z, 0-9, dash (-)",
	},
	{
		"b2:bucketname/prefix/directory/",
		"bucket name contains invalid characters, allowed are: a-z, 0-9, dash (-)",
	},
}

func TestInvalidConfig(t *testing.T) {
	for _, test := range invalidConfigTests {
		t.Run("", func(t *testing.T) {
			cfg, err := ParseConfig(test.s)
			if err == nil {
				t.Fatalf("expected error not found for invalid config: %v, cfg is:\n%#v", test.s, cfg)
			}

			if err.Error() != test.err {
				t.Fatalf("unexpected error found, want:\n  %v\ngot:\n  %v", test.err, err.Error())
			}
		})
	}
}
