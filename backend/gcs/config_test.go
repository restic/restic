package gcs

import "testing"

var configTests = []struct {
	s   string
	cfg Config
}{
	{"gs://bucketname", Config{
		Endpoint: "storage.googleapis.com",
		Bucket:   "bucketname",
		Prefix:   "restic",
	}},
	{"gs://bucketname/", Config{
		Endpoint: "storage.googleapis.com",
		Bucket:   "bucketname",
		Prefix:   "restic",
	}},
	{"gs://bucketname/prefix/dir", Config{
		Endpoint: "storage.googleapis.com",
		Bucket:   "bucketname",
		Prefix:   "prefix/dir",
	}},
	{"gs://bucketname/prefix/dir/", Config{
		Endpoint: "storage.googleapis.com",
		Bucket:   "bucketname",
		Prefix:   "prefix/dir",
	}},
	{"gs:bucketname", Config{
		Endpoint: "storage.googleapis.com",
		Bucket:   "bucketname",
		Prefix:   "restic",
	}},
	{"gs:bucketname/", Config{
		Endpoint: "storage.googleapis.com",
		Bucket:   "bucketname",
		Prefix:   "restic",
	}},
	{"gs:bucketname/prefix/dir", Config{
		Endpoint: "storage.googleapis.com",
		Bucket:   "bucketname",
		Prefix:   "prefix/dir",
	}},
	{"gs:bucketname/prefix/dir/", Config{
		Endpoint: "storage.googleapis.com",
		Bucket:   "bucketname",
		Prefix:   "prefix/dir",
	}},
}

func TestParseConfig(t *testing.T) {
	for i, test := range configTests {
		cfg, err := ParseConfig(test.s)
		if err != nil {
			t.Errorf("test %d failed: %v", i, err)
			continue
		}

		if cfg != test.cfg {
			t.Errorf("test %d: wrong config, want:\n  %v\ngot:\n  %v",
				i, test.cfg, cfg)
			continue
		}
	}
}
