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
	{
		s: "rest:http://localhost:1234",
		cfg: Config{
			URL:         parseURL("http://localhost:1234/"),
			Connections: 5,
		},
	},
	{
		s: "rest:http://localhost:1234/",
		cfg: Config{
			URL:         parseURL("http://localhost:1234/"),
			Connections: 5,
		},
	},
}

func TestParseConfig(t *testing.T) {
	for _, test := range configTests {
		t.Run("", func(t *testing.T) {
			cfg, err := ParseConfig(test.s)
			if err != nil {
				t.Fatalf("%s failed: %v", test.s, err)
			}

			if !reflect.DeepEqual(cfg, test.cfg) {
				t.Fatalf("\ninput: %s\n wrong config, want:\n  %v\ngot:\n  %v",
					test.s, test.cfg, cfg)
			}
		})
	}
}
