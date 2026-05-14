package sftp

import (
	"testing"

	"github.com/restic/restic/internal/backend/test"
)

var configTests = []test.ConfigTestData[Config]{
	// first form, user specified sftp://user@host/dir
	{
		S: "sftp://user@host/dir/subdir",
		Cfg: Config{
			User: "user", Host: "host", Path: "dir/subdir",
			Connections:         defaultConfig.Connections,
			ServerAliveInterval: defaultConfig.ServerAliveInterval,
			ServerAliveCountMax: defaultConfig.ServerAliveCountMax},
	},
	{
		S: "sftp://host/dir/subdir",
		Cfg: Config{
			Host: "host", Path: "dir/subdir",
			Connections:         defaultConfig.Connections,
			ServerAliveInterval: defaultConfig.ServerAliveInterval,
			ServerAliveCountMax: defaultConfig.ServerAliveCountMax},
	},
	{
		S: "sftp://host//dir/subdir",
		Cfg: Config{
			Host: "host", Path: "/dir/subdir",
			Connections:         defaultConfig.Connections,
			ServerAliveInterval: defaultConfig.ServerAliveInterval,
			ServerAliveCountMax: defaultConfig.ServerAliveCountMax},
	},
	{
		S: "sftp://host:10022//dir/subdir",
		Cfg: Config{
			Host: "host", Port: "10022", Path: "/dir/subdir",
			Connections:         defaultConfig.Connections,
			ServerAliveInterval: defaultConfig.ServerAliveInterval,
			ServerAliveCountMax: defaultConfig.ServerAliveCountMax},
	},
	{
		S: "sftp://user@host:10022//dir/subdir",
		Cfg: Config{
			User: "user", Host: "host", Port: "10022", Path: "/dir/subdir",
			Connections:         defaultConfig.Connections,
			ServerAliveInterval: defaultConfig.ServerAliveInterval,
			ServerAliveCountMax: defaultConfig.ServerAliveCountMax},
	},
	{
		S: "sftp://user@host/dir/subdir/../other",
		Cfg: Config{
			User: "user", Host: "host", Path: "dir/other",
			Connections:         defaultConfig.Connections,
			ServerAliveInterval: defaultConfig.ServerAliveInterval,
			ServerAliveCountMax: defaultConfig.ServerAliveCountMax},
	},
	{
		S: "sftp://user@host/dir///subdir",
		Cfg: Config{
			User: "user", Host: "host", Path: "dir/subdir",
			Connections:         defaultConfig.Connections,
			ServerAliveInterval: defaultConfig.ServerAliveInterval,
			ServerAliveCountMax: defaultConfig.ServerAliveCountMax},
	},

	// IPv6 address.
	{
		S: "sftp://user@[::1]/dir",
		Cfg: Config{
			User: "user", Host: "::1", Path: "dir",
			Connections:         defaultConfig.Connections,
			ServerAliveInterval: defaultConfig.ServerAliveInterval,
			ServerAliveCountMax: defaultConfig.ServerAliveCountMax},
	},
	// IPv6 address with port.
	{
		S: "sftp://user@[::1]:22/dir",
		Cfg: Config{
			User: "user", Host: "::1", Port: "22", Path: "dir",
			Connections:         defaultConfig.Connections,
			ServerAliveInterval: defaultConfig.ServerAliveInterval,
			ServerAliveCountMax: defaultConfig.ServerAliveCountMax},
	},

	// second form, user specified sftp:user@host:/dir
	{
		S: "sftp:user@host:/dir/subdir",
		Cfg: Config{
			User: "user", Host: "host", Path: "/dir/subdir",
			Connections:         defaultConfig.Connections,
			ServerAliveInterval: defaultConfig.ServerAliveInterval,
			ServerAliveCountMax: defaultConfig.ServerAliveCountMax},
	},
	{
		S: "sftp:user@domain@host:/dir/subdir",
		Cfg: Config{
			User: "user@domain", Host: "host", Path: "/dir/subdir",
			Connections:         defaultConfig.Connections,
			ServerAliveInterval: defaultConfig.ServerAliveInterval,
			ServerAliveCountMax: defaultConfig.ServerAliveCountMax},
	},
	{
		S: "sftp:host:../dir/subdir",
		Cfg: Config{
			Host: "host", Path: "../dir/subdir",
			Connections:         defaultConfig.Connections,
			ServerAliveInterval: defaultConfig.ServerAliveInterval,
			ServerAliveCountMax: defaultConfig.ServerAliveCountMax},
	},
	{
		S: "sftp:user@host:dir/subdir:suffix",
		Cfg: Config{
			User: "user", Host: "host", Path: "dir/subdir:suffix",
			Connections:         defaultConfig.Connections,
			ServerAliveInterval: defaultConfig.ServerAliveInterval,
			ServerAliveCountMax: defaultConfig.ServerAliveCountMax},
	},
	{
		S: "sftp:user@host:dir/subdir/../other",
		Cfg: Config{
			User: "user", Host: "host", Path: "dir/other",
			Connections:         defaultConfig.Connections,
			ServerAliveInterval: defaultConfig.ServerAliveInterval,
			ServerAliveCountMax: defaultConfig.ServerAliveCountMax},
	},
	{
		S: "sftp:user@host:dir///subdir",
		Cfg: Config{
			User: "user", Host: "host", Path: "dir/subdir",
			Connections:         defaultConfig.Connections,
			ServerAliveInterval: defaultConfig.ServerAliveInterval,
			ServerAliveCountMax: defaultConfig.ServerAliveCountMax},
	},
}

func TestParseConfig(t *testing.T) {
	test.ParseConfigTester(t, ParseConfig, configTests)
}

var configTestsInvalid = []string{
	"sftp://host:dir",
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
