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
		[]string{"host", "-l", "user", "-s", "sftp"},
	},
	{
		Config{Host: "host", Path: "dir/subdir"},
		"ssh",
		[]string{"host", "-s", "sftp"},
	},
	{
		Config{Host: "host:10022", Path: "/dir/subdir"},
		"ssh",
		[]string{"host", "-p", "10022", "-s", "sftp"},
	},
	{
		Config{User: "user", Host: "host:10022", Path: "/dir/subdir"},
		"ssh",
		[]string{"host", "-p", "10022", "-l", "user", "-s", "sftp"},
	},
}

func TestBuildSSHCommand(t *testing.T) {
	for _, test := range sshcmdTests {
		t.Run("", func(t *testing.T) {
			cmd, args, err := buildSSHCommand(test.cfg)
			if err != nil {
				t.Fatal(err)
			}

			if cmd != test.cmd {
				t.Fatalf("cmd: want %v, got %v", test.cmd, cmd)
			}

			if !reflect.DeepEqual(test.args, args) {
				t.Fatalf("wrong args, want:\n  %v\ngot:\n  %v", test.args, args)
			}
		})
	}
}
