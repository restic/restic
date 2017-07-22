package azure

import "testing"

var configTests = []struct {
	s   string
	cfg Config
}{
	{"azure:container-name:/", Config{
		Container:   "container-name",
		Prefix:      "",
		Connections: 5,
	}},
	{"azure:container-name:/prefix/directory", Config{
		Container:   "container-name",
		Prefix:      "prefix/directory",
		Connections: 5,
	}},
	{"azure:container-name:/prefix/directory/", Config{
		Container:   "container-name",
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
