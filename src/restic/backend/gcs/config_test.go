package gcs

import (
	"restic/backend/s3"
	"testing"
)

var configTests = []struct {
	s   string
	cfg s3.Config
}{
	{"gs://bucketname", s3.Config{
		Endpoint:    "storage.googleapis.com",
		Bucket:      "bucketname",
		Prefix:      "restic",
		LocalLayout: true,
	}},
	{"gs://bucketname/", s3.Config{
		Endpoint:    "storage.googleapis.com",
		Bucket:      "bucketname",
		Prefix:      "restic",
		LocalLayout: true,
	}},
	{"gs://bucketname/prefix/dir", s3.Config{
		Endpoint:    "storage.googleapis.com",
		Bucket:      "bucketname",
		Prefix:      "prefix/dir",
		LocalLayout: true,
	}},
	{"gs://bucketname/prefix/dir/", s3.Config{
		Endpoint:    "storage.googleapis.com",
		Bucket:      "bucketname",
		Prefix:      "prefix/dir",
		LocalLayout: true,
	}},
	{"gs:bucketname", s3.Config{
		Endpoint:    "storage.googleapis.com",
		Bucket:      "bucketname",
		Prefix:      "restic",
		LocalLayout: true,
	}},
	{"gs:bucketname/", s3.Config{
		Endpoint:    "storage.googleapis.com",
		Bucket:      "bucketname",
		Prefix:      "restic",
		LocalLayout: true,
	}},
	{"gs:bucketname/prefix/dir", s3.Config{
		Endpoint:    "storage.googleapis.com",
		Bucket:      "bucketname",
		Prefix:      "prefix/dir",
		LocalLayout: true,
	}},
	{"gs:bucketname/prefix/dir/", s3.Config{
		Endpoint:    "storage.googleapis.com",
		Bucket:      "bucketname",
		Prefix:      "prefix/dir",
		LocalLayout: true,
	}},
	{"gs:https://hostname:9999/foobar", s3.Config{
		Endpoint:    "hostname:9999",
		Bucket:      "foobar",
		Prefix:      "restic",
		LocalLayout: true,
	}},
	{"gs:https://hostname:9999/foobar/", s3.Config{
		Endpoint:    "hostname:9999",
		Bucket:      "foobar",
		Prefix:      "restic",
		LocalLayout: true,
	}},
	{"gs:http://hostname:9999/foobar", s3.Config{
		Endpoint:    "hostname:9999",
		Bucket:      "foobar",
		Prefix:      "restic",
		UseHTTP:     true,
		LocalLayout: true,
	}},
	{"gs:http://hostname:9999/foobar/", s3.Config{
		Endpoint:    "hostname:9999",
		Bucket:      "foobar",
		Prefix:      "restic",
		UseHTTP:     true,
		LocalLayout: true,
	}},
	{"gs:http://hostname:9999/bucket/prefix/directory", s3.Config{
		Endpoint:    "hostname:9999",
		Bucket:      "bucket",
		Prefix:      "prefix/directory",
		UseHTTP:     true,
		LocalLayout: true,
	}},
	{"gs:http://hostname:9999/bucket/prefix/directory/", s3.Config{
		Endpoint:    "hostname:9999",
		Bucket:      "bucket",
		Prefix:      "prefix/directory",
		UseHTTP:     true,
		LocalLayout: true,
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
