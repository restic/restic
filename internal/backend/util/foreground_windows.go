package util

import (
	"os/exec"
	"syscall"

	"github.com/restic/restic/internal/errors"
	"golang.org/x/sys/windows"
)

func startForeground(cmd *exec.Cmd) (bg func() error, err error) {
	// just start the process and hope for the best
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.CreationFlags = windows.CREATE_NEW_PROCESS_GROUP
	err = cmd.Start()
	if err != nil {
		return nil, errors.Wrap(err, "cmd.Start")
	}

	bg = func() error { return nil }
	return bg, nil
}
