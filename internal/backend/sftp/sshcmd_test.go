package sftp

import (
	"reflect"
	"testing"
)

var sshcmdTests = []struct {
	cfg  Config
	cmd  string
	args []string
	err  string
}{
	{
		Config{User: "user", Host: "host", Path: "dir/subdir"},
		"ssh",
		[]string{"host", "-l", "user", "-s", "sftp"},
		"",
	},
	{
		Config{Host: "host", Path: "dir/subdir"},
		"ssh",
		[]string{"host", "-s", "sftp"},
		"",
	},
	{
		Config{Host: "host", Port: "10022", Path: "/dir/subdir"},
		"ssh",
		[]string{"host", "-p", "10022", "-s", "sftp"},
		"",
	},
	{
		Config{User: "user", Host: "host", Port: "10022", Path: "/dir/subdir"},
		"ssh",
		[]string{"host", "-p", "10022", "-l", "user", "-s", "sftp"},
		"",
	},
	{
		Config{User: "user", Host: "host", Port: "10022", Path: "/dir/subdir", Args: "-i /path/to/id_rsa"},
		"ssh",
		[]string{"host", "-p", "10022", "-l", "user", "-i", "/path/to/id_rsa", "-s", "sftp"},
		"",
	},
	{
		Config{Command: "ssh something", Args: "-i /path/to/id_rsa"},
		"",
		nil,
		"cannot specify both sftp.command and sftp.args options",
	},
	{
		// IPv6 address.
		Config{User: "user", Host: "::1", Path: "dir"},
		"ssh",
		[]string{"::1", "-l", "user", "-s", "sftp"},
		"",
	},
	{
		// IPv6 address with zone and port.
		Config{User: "user", Host: "::1%lo0", Port: "22", Path: "dir"},
		"ssh",
		[]string{"::1%lo0", "-p", "22", "-l", "user", "-s", "sftp"},
		"",
	},
}

func TestBuildSSHCommand(t *testing.T) {
	for i, test := range sshcmdTests {
		t.Run("", func(t *testing.T) {
			cmd, args, err := buildSSHCommand(test.cfg)
			if test.err != "" {
				if err.Error() != test.err {
					t.Fatalf("expected error %v got %v", test.err, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("%v in test %d", err, i)
				}
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
