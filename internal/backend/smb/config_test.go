package smb

import (
	"strings"
	"testing"
)

var configTests = []struct {
	s   string
	cfg Config
}{
	{"smb://shareaddress/sharename/directory", Config{
		Address:     "shareaddress",
		Port:        DefaultSmbPort,
		ShareName:   "sharename",
		Path:        "directory",
		Domain:      DefaultDomain,
		Connections: DefaultConnections,
		IdleTimeout: DefaultIdleTimeout,
	}},
	{"smb://shareaddress:456/sharename/directory", Config{
		Address:     "shareaddress",
		Port:        456,
		ShareName:   "sharename",
		Path:        "directory",
		Domain:      DefaultDomain,
		Connections: DefaultConnections,
		IdleTimeout: DefaultIdleTimeout,
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

func TestParseError(t *testing.T) {
	const prefix = "smb: invalid format,"

	for _, s := range []string{"", "/", "//", "/sharename/directory"} {
		_, err := ParseConfig("smb://" + s)
		if err == nil || !strings.HasPrefix(err.Error(), prefix) {
			t.Errorf("expected %q, got %q", prefix, err)
		}
	}
}
