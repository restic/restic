package backend

import (
	"os/exec"
	"syscall"

	"github.com/restic/restic/internal/errors"
)

// StartForeground runs cmd in the foreground, by temporarily switching to the
// new process group created for cmd. The returned function `bg` switches back
// to the previous process group.
func StartForeground(cmd *exec.Cmd) (bg func() error, err error) {
	// run the command in it's own process group so that SIGINT
	// is not sent to it.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// start the process
	err = cmd.Start()
	if err != nil {
		return nil, errors.Wrap(err, "cmd.Start")
	}

	bg = func() error { return nil }
	return bg, nil
}
