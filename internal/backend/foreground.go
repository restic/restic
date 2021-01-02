package backend

import (
	"os"
	"os/exec"
	"strings"
)

// StartForeground runs cmd in the foreground, by temporarily switching to the
// new process group created for cmd. The returned function `bg` switches back
// to the previous process group.
//
// The command's environment has all RESTIC_* variables removed.
func StartForeground(cmd *exec.Cmd) (bg func() error, err error) {
	env := os.Environ() // Returns a copy that we can modify.

	cmd.Env = env[:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, "RESTIC_") {
			continue
		}
		cmd.Env = append(cmd.Env, kv)
	}

	return startForeground(cmd)
}
