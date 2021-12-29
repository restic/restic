package backend

import (
	"os/exec"
	"syscall"

	"github.com/restic/restic/internal/errors"
	"golang.org/x/sys/windows"
)

func startForeground(cmd *exec.Cmd) (bg func() error, err error) {
	// just start the process and hope for the best
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.CreationFlags = windows.DETACHED_PROCESS
	err = cmd.Start()
	if err != nil {
		return nil, errors.Wrap(err, "cmd.Start")
	}

	bg = func() error { return nil }
	return bg, nil
}
