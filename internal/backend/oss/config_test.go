package oss

import "testing"

var configTests = []struct {
	s   string
	cfg Config
}{
	{"oss:::::", Config{
		Host:        "",
		AccessID:    "",
		AccessKey:   "",
		Bucket:      "",
		Prefix:      "",
		Connections: 40,
	}},
	{"oss:oss.aliyuncs.com::::", Config{
		Host:        "oss.aliyuncs.com",
		AccessID:    "",
		AccessKey:   "",
		Bucket:      "",
		Prefix:      "",
		Connections: 40,
	}},
	{"oss:oss.aliyuncs.com:OsjvNEXyrwuou4o1:CZ1WFjvJGDDKXempFdzN369RnB0JApv1::", Config{
		Host:        "oss.aliyuncs.com",
		AccessID:    "OsjvNEXyrwuou4o1",
		AccessKey:   "CZ1WFjvJGDDKXempFdzN369RnB0JApv1",
		Bucket:      "",
		Prefix:      "",
		Connections: 40,
	}},
	{"oss:oss.aliyuncs.com:OsjvNEXyrwuou4o1:CZ1WFjvJGDDKXempFdzN369RnB0JApv1:restic:repo", Config{
		Host:        "oss.aliyuncs.com",
		AccessID:    "OsjvNEXyrwuou4o1",
		AccessKey:   "CZ1WFjvJGDDKXempFdzN369RnB0JApv1",
		Bucket:      "restic",
		Prefix:      "repo",
		Connections: 40,
	}},
}

func TestParseConfig(t *testing.T) {
	for _, test := range configTests {
		t.Run("", func(t *testing.T) {
			cfg, err := ParseConfig(test.s)
			if err != nil {
				t.Fatalf("%s failed: %v", test.s, err)
			}

			if cfg != test.cfg {
				t.Fatalf("input: %s\n wrong config, want:\n  %#v\ngot:\n  %#v",
					test.s, test.cfg, cfg)
			}
		})
	}
}
