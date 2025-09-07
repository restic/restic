//go:build unix

package util

import (
	"os"
	"os/exec"
	"os/signal"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/ui/termstatus"

	"golang.org/x/sys/unix"
)

func startForeground(cmd *exec.Cmd) (bg func() error, err error) {
	// run the command in its own process group
	// this ensures that sending ctrl-c to restic will not immediately stop the backend process.
	cmd.SysProcAttr = &unix.SysProcAttr{
		Setpgid: true,
	}

	// open the TTY, we need the file descriptor
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		debug.Log("unable to open tty: %v", err)
		return startFallback(cmd)
	}

	// only move child process to foreground if restic is in the foreground
	prev, err := termstatus.Tcgetpgrp(int(tty.Fd()))
	if err != nil {
		_ = tty.Close()
		return nil, err
	}

	self := termstatus.Getpgrp()
	if prev != self {
		debug.Log("restic is not controlling the tty; err = %v", err)
		if err := tty.Close(); err != nil {
			return nil, err
		}
		return startFallback(cmd)
	}

	// Prevent getting suspended when interacting with the tty
	signal.Ignore(unix.SIGTTIN)
	signal.Ignore(unix.SIGTTOU)

	// start the process
	err = cmd.Start()
	if err != nil {
		_ = tty.Close()
		return nil, errors.Wrap(err, "cmd.Start")
	}

	// move the command's process group into the foreground
	err = termstatus.Tcsetpgrp(int(tty.Fd()), cmd.Process.Pid)
	if err != nil {
		_ = tty.Close()
		return nil, err
	}

	bg = func() error {
		signal.Reset(unix.SIGTTIN)
		signal.Reset(unix.SIGTTOU)

		// reset the foreground process group
		err = termstatus.Tcsetpgrp(int(tty.Fd()), prev)
		if err != nil {
			_ = tty.Close()
			return err
		}

		return tty.Close()
	}

	return bg, nil
}

func startFallback(cmd *exec.Cmd) (bg func() error, err error) {
	bg = func() error {
		return nil
	}

	return bg, cmd.Start()
}
