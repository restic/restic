package swift

import "testing"

var configTests = []struct {
	s   string
	cfg Config
}{
	{
		"swift:cnt1:/",
		Config{
			Container:   "cnt1",
			Prefix:      "",
			Connections: 5,
		},
	},
	{
		"swift:cnt2:/prefix",
		Config{Container: "cnt2",
			Prefix:      "prefix",
			Connections: 5,
		},
	},
	{
		"swift:cnt3:/prefix/longer",
		Config{Container: "cnt3",
			Prefix:      "prefix/longer",
			Connections: 5,
		},
	},
}

func TestParseConfig(t *testing.T) {
	for _, test := range configTests {
		t.Run("", func(t *testing.T) {
			v, err := ParseConfig(test.s)
			if err != nil {
				t.Fatalf("parsing %q failed: %v", test.s, err)
			}

			cfg, ok := v.(Config)
			if !ok {
				t.Fatalf("wrong type returned, want Config, got %T", cfg)
			}

			if cfg != test.cfg {
				t.Fatalf("wrong output for %q, want:\n  %#v\ngot:\n  %#v",
					test.s, test.cfg, cfg)
			}
		})
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
