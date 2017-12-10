package s3

import "testing"

var configTests = []struct {
	s   string
	cfg Config
}{
	{"s3://eu-central-1/bucketname", Config{
		Endpoint:    "eu-central-1",
		Bucket:      "bucketname",
		Prefix:      "",
		Connections: 5,
	}},
	{"s3://eu-central-1/bucketname/", Config{
		Endpoint:    "eu-central-1",
		Bucket:      "bucketname",
		Prefix:      "",
		Connections: 5,
	}},
	{"s3://eu-central-1/bucketname/prefix/directory", Config{
		Endpoint:    "eu-central-1",
		Bucket:      "bucketname",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
	{"s3://eu-central-1/bucketname/prefix/directory/", Config{
		Endpoint:    "eu-central-1",
		Bucket:      "bucketname",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
	{"s3:eu-central-1/foobar", Config{
		Endpoint:    "eu-central-1",
		Bucket:      "foobar",
		Prefix:      "",
		Connections: 5,
	}},
	{"s3:eu-central-1/foobar/", Config{
		Endpoint:    "eu-central-1",
		Bucket:      "foobar",
		Prefix:      "",
		Connections: 5,
	}},
	{"s3:eu-central-1/foobar/prefix/directory", Config{
		Endpoint:    "eu-central-1",
		Bucket:      "foobar",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
	{"s3:eu-central-1/foobar/prefix/directory/", Config{
		Endpoint:    "eu-central-1",
		Bucket:      "foobar",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
	{"s3:https://hostname:9999/foobar", Config{
		Endpoint:    "hostname:9999",
		Bucket:      "foobar",
		Prefix:      "",
		Connections: 5,
	}},
	{"s3:https://hostname:9999/foobar/", Config{
		Endpoint:    "hostname:9999",
		Bucket:      "foobar",
		Prefix:      "",
		Connections: 5,
	}},
	{"s3:http://hostname:9999/foobar", Config{
		Endpoint:    "hostname:9999",
		Bucket:      "foobar",
		Prefix:      "",
		UseHTTP:     true,
		Connections: 5,
	}},
	{"s3:http://hostname:9999/foobar/", Config{
		Endpoint:    "hostname:9999",
		Bucket:      "foobar",
		Prefix:      "",
		UseHTTP:     true,
		Connections: 5,
	}},
	{"s3:http://hostname:9999/bucket/prefix/directory", Config{
		Endpoint:    "hostname:9999",
		Bucket:      "bucket",
		Prefix:      "prefix/directory",
		UseHTTP:     true,
		Connections: 5,
	}},
	{"s3:http://hostname:9999/bucket/prefix/directory/", Config{
		Endpoint:    "hostname:9999",
		Bucket:      "bucket",
		Prefix:      "prefix/directory",
		UseHTTP:     true,
		Connections: 5,
	}},
}

func TestParseConfig(t *testing.T) {
	for i, test := range configTests {
		cfg, err := ParseConfig(test.s)
		if err != nil {
			t.Errorf("test %d:%s failed: %v", i, test.s, err)
			continue
		}

		if cfg != test.cfg {
			t.Errorf("test %d:\ninput:\n  %s\n wrong config, want:\n  %v\ngot:\n  %v",
				i, test.s, test.cfg, cfg)
			continue
		}
	}
}
