package swift

import "testing"

var configTests = []struct {
	s   string
	cfg Config
}{
	{"swift:cnt1:/", Config{Container: "cnt1", Prefix: ""}},
	{"swift:cnt2:/prefix", Config{Container: "cnt2", Prefix: "prefix"}},
	{"swift:cnt3:/prefix/longer", Config{Container: "cnt3", Prefix: "prefix/longer"}},
	{"swift:cnt4:/prefix?params", Config{Container: "cnt4", Prefix: "prefix"}},
	{"swift:cnt5:/prefix#params", Config{Container: "cnt5", Prefix: "prefix"}},
}

func TestParseConfigInternal(t *testing.T) {
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

var configTestsInvalid = []string{
	"swift://hostname/container",
	"swift:////",
	"swift://",
	"swift:////prefix",
	"swift:container",
	"swift:container:",
	"swift:container/prefix",
}

func TestParseConfigInvalid(t *testing.T) {
	for i, test := range configTestsInvalid {
		_, err := ParseConfig(test)
		if err == nil {
			t.Errorf("test %d: invalid config %s did not return an error", i, test)
			continue
		}
	}
}
