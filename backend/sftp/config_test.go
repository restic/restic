package sftp

import "testing"

var configTests = []struct {
	s   string
	cfg Config
}{
	// first form, user specified sftp://user@host/dir
	{
		"sftp://user@host/dir/subdir",
		Config{User: "user", Host: "host", Dir: "dir/subdir"},
	},
	{
		"sftp://host/dir/subdir",
		Config{Host: "host", Dir: "dir/subdir"},
	},
	{
		"sftp://host//dir/subdir",
		Config{Host: "host", Dir: "/dir/subdir"},
	},
	{
		"sftp://host:10022//dir/subdir",
		Config{Host: "host:10022", Dir: "/dir/subdir"},
	},
	{
		"sftp://user@host:10022//dir/subdir",
		Config{User: "user", Host: "host:10022", Dir: "/dir/subdir"},
	},

	// second form, user specified sftp:user@host:/dir
	{
		"sftp:foo@bar:/baz/quux",
		Config{User: "foo", Host: "bar", Dir: "/baz/quux"},
	},
	{
		"sftp:bar:../baz/quux",
		Config{Host: "bar", Dir: "../baz/quux"},
	},
	{
		"sftp:fux@bar:baz/qu:ux",
		Config{User: "fux", Host: "bar", Dir: "baz/qu:ux"},
	},
}

func TestParseConfig(t *testing.T) {
	for i, test := range configTests {
		cfg, err := ParseConfig(test.s)
		if err != nil {
			t.Errorf("test %d failed: %v", i, err)
			continue
		}

		if cfg != test.cfg {
			t.Errorf("test %d: wrong config, want:\n  %v\ngot:\n  %v",
				i, test.cfg, cfg)
			continue
		}
	}
}
