package sftp

import "testing"

var sshcmdTests = []struct {
	cfg Config
	s   []string
}{
	{
		Config{User: "user", Host: "host", Dir: "dir/subdir"},
		[]string{"host", "-l", "user", "-s", "sftp"},
	},
	{
		Config{Host: "host", Dir: "dir/subdir"},
		[]string{"host", "-s", "sftp"},
	},
	{
		Config{Host: "host:10022", Dir: "/dir/subdir"},
		[]string{"host", "-p", "10022", "-s", "sftp"},
	},
	{
		Config{User: "user", Host: "host:10022", Dir: "/dir/subdir"},
		[]string{"host", "-p", "10022", "-l", "user", "-s", "sftp"},
	},
}

func TestBuildSSHCommand(t *testing.T) {
	for i, test := range sshcmdTests {
		cmd := buildSSHCommand(test.cfg)
		failed := false
		if len(cmd) != len(test.s) {
			failed = true
		} else {
			for l := range test.s {
				if test.s[l] != cmd[l] {
					failed = true
					break
				}
			}
		}
		if failed {
			t.Errorf("test %d: wrong cmd, want:\n  %v\ngot:\n  %v",
				i, test.s, cmd)
		}
	}
}
