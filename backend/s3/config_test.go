package s3

import "testing"

var configTests = []struct {
	s   string
	cfg Config
}{
	{"s3://eu-central-1/bucketname", Config{
		Region: "eu-central-1",
		Bucket: "bucketname",
	}},
	{"s3:eu-central-1/foobar", Config{
		Region: "eu-central-1",
		Bucket: "foobar",
	}},
	{"s3:https://hostname:9999/foobar", Config{
		URL:    "https://hostname:9999",
		Bucket: "foobar",
	}},
	{"s3:http://hostname:9999/foobar", Config{
		URL:    "http://hostname:9999",
		Bucket: "foobar",
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
