//go:build !aix && !solaris && !windows
// +build !aix,!solaris,!windows

package util

import (
	"os"
	"os/exec"
	"os/signal"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"

	"golang.org/x/sys/unix"
)

func tcsetpgrp(fd int, pid int) error {
	// IoctlSetPointerInt silently casts to int32 internally,
	// so this assumes pid fits in 31 bits.
	return unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, pid)
}

func startForeground(cmd *exec.Cmd) (bg func() error, err error) {
	// open the TTY, we need the file descriptor
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		debug.Log("unable to open tty: %v", err)
		bg = func() error {
			return nil
		}
		return bg, cmd.Start()
	}

	signal.Ignore(unix.SIGTTIN)
	signal.Ignore(unix.SIGTTOU)

	// run the command in its own process group
	cmd.SysProcAttr = &unix.SysProcAttr{
		Setpgid: true,
	}

	// start the process
	err = cmd.Start()
	if err != nil {
		_ = tty.Close()
		return nil, errors.Wrap(err, "cmd.Start")
	}

	// move the command's process group into the foreground
	prev := unix.Getpgrp()
	err = tcsetpgrp(int(tty.Fd()), cmd.Process.Pid)
	if err != nil {
		_ = tty.Close()
		return nil, err
	}

	bg = func() error {
		signal.Reset(unix.SIGTTIN)
		signal.Reset(unix.SIGTTOU)

		// reset the foreground process group
		err = tcsetpgrp(int(tty.Fd()), prev)
		if err != nil {
			_ = tty.Close()
			return err
		}

		return tty.Close()
	}

	return bg, nil
}
