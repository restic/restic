//go:build aix || solaris
// +build aix solaris

package util

import (
	"os/exec"
	"syscall"

	"github.com/restic/restic/internal/errors"
)

func startForeground(cmd *exec.Cmd) (bg func() error, err error) {
	// run the command in its own process group so that SIGINT
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
