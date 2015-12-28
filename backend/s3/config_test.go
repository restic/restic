package s3

import "testing"

var configTests = []struct {
	s   string
	cfg Config
}{
	{"s3://eu-central-1/bucketname", Config{
		Host:   "eu-central-1",
		Bucket: "bucketname",
	}},
	{"s3:hostname:foobar", Config{
		Host:   "hostname",
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
