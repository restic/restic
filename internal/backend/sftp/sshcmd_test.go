package sftp

import (
	"reflect"
	"testing"
)

var sshcmdTests = []struct {
	cfg  Config
	cmd  string
	args []string
}{
	{
		Config{User: "user", Host: "host", Path: "dir/subdir"},
		"ssh",
		[]string{"-l", "user", "-s", "sftp", "--", "host"},
	},
	{
		Config{Host: "host", Path: "dir/subdir"},
		"ssh",
		[]string{"-s", "sftp", "--", "host"},
	},
	{
		Config{Host: "host", Port: "10022", Path: "/dir/subdir"},
		"ssh",
		[]string{"-p", "10022", "-s", "sftp", "--", "host"},
	},
	{
		Config{User: "user", Host: "host", Port: "10022", Path: "/dir/subdir"},
		"ssh",
		[]string{"-p", "10022", "-l", "user", "-s", "sftp", "--", "host"},
	},
	{
		// IPv6 address.
		Config{User: "user", Host: "::1", Path: "dir"},
		"ssh",
		[]string{"-l", "user", "-s", "sftp", "--", "::1"},
	},
	{
		// IPv6 address with zone and port.
		Config{User: "user", Host: "::1%lo0", Port: "22", Path: "dir"},
		"ssh",
		[]string{"-p", "22", "-l", "user", "-s", "sftp", "--", "::1%lo0"},
	},
}

func TestBuildSSHCommand(t *testing.T) {
	for i, test := range sshcmdTests {
		t.Run("", func(t *testing.T) {
			cmd, args, err := buildSSHCommand(test.cfg)
			if err != nil {
				t.Fatalf("%v in test %d", err, i)
			}

			if cmd != test.cmd {
				t.Fatalf("cmd: want %v, got %v", test.cmd, cmd)
			}

			if !reflect.DeepEqual(test.args, args) {
				t.Fatalf("wrong args in test %d, want:\n  %v\ngot:\n  %v",
					i, test.args, args)
			}
		})
	}
}
