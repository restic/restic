package rclone

import (
	"reflect"
	"testing"
)

func TestParseConfig(t *testing.T) {
	var tests = []struct {
		s   string
		cfg Config
	}{
		{
			"rclone:local:foo:/bar",
			Config{
				Remote: "local:foo:/bar",
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			cfg, err := ParseConfig(test.s)
			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(cfg, test.cfg) {
				t.Fatalf("wrong config, want:\n  %v\ngot:\n  %v", test.cfg, cfg)
			}
		})
	}
}
