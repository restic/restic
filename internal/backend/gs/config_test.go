package gs

import "testing"

var configTests = []struct {
	s   string
	cfg Config
}{
	{"gs:bucketname:/", Config{
		Bucket:      "bucketname",
		Prefix:      "",
		Connections: 5,
	}},
	{"gs:bucketname:/prefix/directory", Config{
		Bucket:      "bucketname",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
	{"gs:bucketname:/prefix/directory/", Config{
		Bucket:      "bucketname",
		Prefix:      "prefix/directory",
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

		if (cfg.(Config).Bucket != test.cfg.Bucket) || (cfg.(Config).Prefix != test.cfg.Prefix) || (cfg.(Config).Connections != test.cfg.Connections) {
			t.Errorf("test %d:\ninput:\n  %s\n wrong config, want:\n  %v\ngot:\n  %v",
				i, test.s, test.cfg, cfg)
			continue
		}
	}
}
