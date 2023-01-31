package smb

import (
	"strings"
	"testing"
)

var configTests = []struct {
	s   string
	cfg Config
}{
	{"smb://user@host/sharename/directory", Config{
		Host:        "host",
		Port:        DefaultSmbPort,
		User:        "user",
		Domain:      DefaultDomain,
		ShareName:   "sharename",
		Path:        "directory",
		Connections: DefaultConnections,
		IdleTimeout: DefaultIdleTimeout,
	}},
	{"smb://user@host:456/sharename/directory", Config{
		Host:        "host",
		Port:        456,
		User:        "user",
		Domain:      DefaultDomain,
		ShareName:   "sharename",
		Path:        "directory",
		Connections: DefaultConnections,
		IdleTimeout: DefaultIdleTimeout,
	}},
	{"smb://host/sharename/directory", Config{
		Host:        "host",
		Port:        DefaultSmbPort,
		Domain:      DefaultDomain,
		ShareName:   "sharename",
		Path:        "directory",
		Connections: DefaultConnections,
		IdleTimeout: DefaultIdleTimeout,
	}},
	{"smb://host:446/sharename/directory", Config{
		Host:        "host",
		Port:        446,
		Domain:      DefaultDomain,
		ShareName:   "sharename",
		Path:        "directory",
		Connections: DefaultConnections,
		IdleTimeout: DefaultIdleTimeout,
	}},
	{"smb:user@host:466/sharename/directory", Config{
		Host:        "host",
		Port:        466,
		User:        "user",
		Domain:      DefaultDomain,
		ShareName:   "sharename",
		Path:        "directory",
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

	for _, s := range []string{"", "/", "//", "host", "user@host", "user@host:445", "/sharename/directory"} {
		_, err := ParseConfig("smb://" + s)
		if err == nil || !strings.HasPrefix(err.Error(), prefix) {
			t.Errorf("expected %q, got %q", prefix, err)
		}
	}
}
