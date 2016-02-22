package rest

import (
	"net/url"
	"reflect"
	"testing"
)

func parseURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}

	return u
}

var configTests = []struct {
	s   string
	cfg Config
}{
	{"rest:http://localhost:1234", Config{
		URL: parseURL("http://localhost:1234"),
	}},
}

func TestParseConfig(t *testing.T) {
	for i, test := range configTests {
		cfg, err := ParseConfig(test.s)
		if err != nil {
			t.Errorf("test %d:%s failed: %v", i, test.s, err)
			continue
		}

		if !reflect.DeepEqual(cfg, test.cfg) {
			t.Errorf("test %d:\ninput:\n  %s\n wrong config, want:\n  %v\ngot:\n  %v",
				i, test.s, test.cfg, cfg)
			continue
		}
	}
}
