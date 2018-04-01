package backend

import (
	"os/exec"

	"github.com/restic/restic/internal/errors"
)

// StartForeground runs cmd in the foreground, by temporarily switching to the
// new process group created for cmd. The returned function `bg` switches back
// to the previous process group.
func StartForeground(cmd *exec.Cmd) (bg func() error, err error) {
	// just start the process and hope for the best
	err = cmd.Start()
	if err != nil {
		return nil, errors.Wrap(err, "cmd.Start")
	}

	bg = func() error { return nil }
	return bg, nil
}
