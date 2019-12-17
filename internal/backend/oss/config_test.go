package oss

import "testing"

var configTests = []struct {
	s   string
	cfg Config
}{
	{"oss:oss-cn-hangzhou.aliyuncs.com/bucketname", Config{
		Endpoint:    "oss-cn-hangzhou.aliyuncs.com",
		Bucket:      "bucketname",
		Prefix:      "",
		Connections: 5,
	}},
	{"oss:oss-cn-hangzhou.aliyuncs.com/bucketname/prefix/directory", Config{
		Endpoint:    "oss-cn-hangzhou.aliyuncs.com",
		Bucket:      "bucketname",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
	{"oss:oss-cn-hangzhou.aliyuncs.com/bucketname/prefix/directory/", Config{
		Endpoint:    "oss-cn-hangzhou.aliyuncs.com",
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

		if cfg != test.cfg {
			t.Errorf("test %d:\ninput:\n  %s\n wrong config, want:\n  %v\ngot:\n  %v",
				i, test.s, test.cfg, cfg)
			continue
		}
	}
}
