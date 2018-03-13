// +build !solaris
// +build !windows

package backend

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"unsafe"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
)

func tcsetpgrp(fd int, pid int) error {
	_, _, errno := syscall.RawSyscall(syscall.SYS_IOCTL, uintptr(fd),
		uintptr(syscall.TIOCSPGRP), uintptr(unsafe.Pointer(&pid)))
	if errno == 0 {
		return nil
	}

	return errno
}

// StartForeground runs cmd in the foreground, by temporarily switching to the
// new process group created for cmd. The returned function `bg` switches back
// to the previous process group.
func StartForeground(cmd *exec.Cmd) (bg func() error, err error) {
	// open the TTY, we need the file descriptor
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		debug.Log("unable to open tty: %v", err)
		bg = func() error {
			return nil
		}
		return bg, cmd.Start()
	}

	signal.Ignore(syscall.SIGTTIN)
	signal.Ignore(syscall.SIGTTOU)

	// run the command in its own process group
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// start the process
	err = cmd.Start()
	if err != nil {
		_ = tty.Close()
		return nil, errors.Wrap(err, "cmd.Start")
	}

	// move the command's process group into the foreground
	prev := syscall.Getpgrp()
	err = tcsetpgrp(int(tty.Fd()), cmd.Process.Pid)
	if err != nil {
		_ = tty.Close()
		return nil, err
	}

	bg = func() error {
		signal.Reset(syscall.SIGTTIN)
		signal.Reset(syscall.SIGTTOU)

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
