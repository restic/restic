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
		Config{User: "user", Host: "host", Path: "dir/subdir", ServerAliveInterval: -1, ServerAliveCountMax: -1},
		"ssh",
		[]string{"host", "-l", "user", "-s", "sftp"},
		"",
	},
	{
		Config{Host: "host", Path: "dir/subdir", ServerAliveInterval: -1, ServerAliveCountMax: -1},
		"ssh",
		[]string{"host", "-s", "sftp"},
		"",
	},
	{
		Config{Host: "host", Port: "10022", Path: "/dir/subdir", ServerAliveInterval: -1, ServerAliveCountMax: -1},
		"ssh",
		[]string{"host", "-p", "10022", "-s", "sftp"},
		"",
	},
	{
		Config{User: "user", Host: "host", Port: "10022", Path: "/dir/subdir", ServerAliveInterval: -1, ServerAliveCountMax: -1},
		"ssh",
		[]string{"host", "-p", "10022", "-l", "user", "-s", "sftp"},
		"",
	},
	{
		Config{User: "user", Host: "host", Port: "10022", Path: "/dir/subdir", Args: "-i /path/to/id_rsa", ServerAliveInterval: -1, ServerAliveCountMax: -1},
		"ssh",
		[]string{"host", "-p", "10022", "-l", "user", "-i", "/path/to/id_rsa", "-s", "sftp"},
		"",
	},
	{
		Config{Command: "ssh something", Args: "-i /path/to/id_rsa", ServerAliveInterval: -1, ServerAliveCountMax: -1},
		"",
		nil,
		"cannot specify both sftp.command and sftp.args options",
	},
	{
		// IPv6 address.
		Config{User: "user", Host: "::1", Path: "dir", ServerAliveInterval: -1, ServerAliveCountMax: -1},
		"ssh",
		[]string{"::1", "-l", "user", "-s", "sftp"},
		"",
	},
	{
		// IPv6 address with zone and port.
		Config{User: "user", Host: "::1%lo0", Port: "22", Path: "dir", ServerAliveInterval: -1, ServerAliveCountMax: -1},
		"ssh",
		[]string{"::1%lo0", "-p", "22", "-l", "user", "-s", "sftp"},
		"",
	},
	{
		Config{User: "user", Host: "host", Path: "dir", ServerAliveInterval: 99, ServerAliveCountMax: -1},
		"ssh",
		[]string{"host", "-l", "user", "-o", "ServerAliveInterval=99", "-s", "sftp"},
		"",
	},
	{
		Config{User: "user", Host: "host", Path: "dir", ServerAliveInterval: -1, ServerAliveCountMax: 99},
		"ssh",
		[]string{"host", "-l", "user", "-o", "ServerAliveCountMax=99", "-s", "sftp"},
		"",
	},
	{
		Config{User: "user", Host: "host", Path: "dir", ServerAliveInterval: 99, ServerAliveCountMax: 99},
		"ssh",
		[]string{"host", "-l", "user", "-o", "ServerAliveInterval=99", "-o", "ServerAliveCountMax=99", "-s", "sftp"},
		"",
	},
	{
		Config{User: "user", Host: "host", Path: "dir", ServerAliveInterval: 99, ServerAliveCountMax: 99, Args: "-i /path/to/id_rsa"},
		"ssh",
		[]string{"host", "-l", "user", "-o", "ServerAliveInterval=99", "-o", "ServerAliveCountMax=99", "-i", "/path/to/id_rsa", "-s", "sftp"},
		"",
	},
	{
		Config{User: "user", Host: "host", Path: "dir", ServerAliveInterval: 0, ServerAliveCountMax: 99},
		"ssh",
		[]string{"host", "-l", "user", "-o", "ServerAliveInterval=0", "-o", "ServerAliveCountMax=99", "-s", "sftp"},
		"",
	},
	{
		Config{User: "user", Host: "host", Path: "dir", ServerAliveInterval: 99, ServerAliveCountMax: 0},
		"",
		nil,
		"sftp.server-alive-count-max cannot be 0",
	},
}

func TestBuildSSHCommand(t *testing.T) {
	for i, test := range sshcmdTests {
		t.Run("", func(t *testing.T) {
			cmd, args, err := buildSSHCommand(test.cfg)
			if test.err != "" {
				if err == nil {
					t.Fatalf("expected error %v got nil", test.err)
				}
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
