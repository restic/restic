package sftp

import (
	"testing"
)

var configTests = []struct {
	in  string
	cfg Config
}{
	// first form, user specified sftp://user@host/dir
	{
		"sftp://user@host/dir/subdir",
		Config{User: "user", Host: "host", Path: "dir/subdir"},
	},
	{
		"sftp://host/dir/subdir",
		Config{Host: "host", Path: "dir/subdir"},
	},
	{
		"sftp://host//dir/subdir",
		Config{Host: "host", Path: "/dir/subdir"},
	},
	{
		"sftp://host:10022//dir/subdir",
		Config{Host: "host:10022", Path: "/dir/subdir"},
	},
	{
		"sftp://user@host:10022//dir/subdir",
		Config{User: "user", Host: "host:10022", Path: "/dir/subdir"},
	},
	{
		"sftp://user@host/dir/subdir/../other",
		Config{User: "user", Host: "host", Path: "dir/other"},
	},
	{
		"sftp://user@host/dir///subdir",
		Config{User: "user", Host: "host", Path: "dir/subdir"},
	},

	// second form, user specified sftp:user@host:/dir
	{
		"sftp:user@host:/dir/subdir",
		Config{User: "user", Host: "host", Path: "/dir/subdir"},
	},
	{
		"sftp:user@domain@host:/dir/subdir",
		Config{User: "user@domain", Host: "host", Path: "/dir/subdir"},
	},
	{
		"sftp:host:../dir/subdir",
		Config{Host: "host", Path: "../dir/subdir"},
	},
	{
		"sftp:user@host:dir/subdir:suffix",
		Config{User: "user", Host: "host", Path: "dir/subdir:suffix"},
	},
	{
		"sftp:user@host:dir/subdir/../other",
		Config{User: "user", Host: "host", Path: "dir/other"},
	},
	{
		"sftp:user@host:dir///subdir",
		Config{User: "user", Host: "host", Path: "dir/subdir"},
	},
}

func TestParseConfig(t *testing.T) {
	for i, test := range configTests {
		cfg, err := ParseConfig(test.in)
		if err != nil {
			t.Errorf("test %d:%s failed: %v", i, test.in, err)
			continue
		}

		if cfg != test.cfg {
			t.Errorf("test %d:\ninput:\n  %s\n wrong config, want:\n  %v\ngot:\n  %v",
				i, test.in, test.cfg, cfg)
			continue
		}
	}
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
