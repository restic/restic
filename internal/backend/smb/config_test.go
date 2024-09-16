package smb

import (
	"strings"
	"testing"

	"github.com/restic/restic/internal/backend/test"
)

var configTests = []test.ConfigTestData[Config]{
	{S: "smb://user@host/sharename/directory",
		Cfg: Config{
			Host:        "host",
			Port:        DefaultSMBPort,
			User:        "user",
			Domain:      DefaultDomain,
			ShareName:   "sharename",
			Path:        "directory",
			Connections: DefaultConnections,
			IdleTimeout: DefaultIdleTimeout,
		}},
	{S: "smb://user@host:456/sharename/directory",
		Cfg: Config{
			Host:        "host",
			Port:        456,
			User:        "user",
			Domain:      DefaultDomain,
			ShareName:   "sharename",
			Path:        "directory",
			Connections: DefaultConnections,
			IdleTimeout: DefaultIdleTimeout,
		}},
	{S: "smb://host/sharename/directory",
		Cfg: Config{
			Host:        "host",
			Port:        DefaultSMBPort,
			Domain:      DefaultDomain,
			ShareName:   "sharename",
			Path:        "directory",
			Connections: DefaultConnections,
			IdleTimeout: DefaultIdleTimeout,
		}},
	{S: "smb://host:446/sharename/directory",
		Cfg: Config{
			Host:        "host",
			Port:        446,
			Domain:      DefaultDomain,
			ShareName:   "sharename",
			Path:        "directory",
			Connections: DefaultConnections,
			IdleTimeout: DefaultIdleTimeout,
		}},
	{S: "smb:user@host:466/sharename/directory",
		Cfg: Config{
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
	test.ParseConfigTester(t, ParseConfig, configTests)
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
