//go:build !windows
// +build !windows

package restic

import (
	"os"
	"os/user"
	"strconv"
	"syscall"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
)

// uidGidInt returns uid, gid of the user as a number.
func uidGidInt(u *user.User) (uid, gid uint32, err error) {
	ui, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return 0, 0, errors.Errorf("invalid UID %q", u.Uid)
	}
	gi, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return 0, 0, errors.Errorf("invalid GID %q", u.Gid)
	}
	return uint32(ui), uint32(gi), nil
}

// checkProcess will check if the process retaining the lock
// exists and responds to SIGHUP signal.
// Returns true if the process exists and responds.
func (l *Lock) processExists() bool {
	proc, err := os.FindProcess(l.PID)
	if err != nil {
		debug.Log("error searching for process %d: %v\n", l.PID, err)
		return false
	}
	defer func() {
		_ = proc.Release()
	}()

	debug.Log("sending SIGHUP to process %d\n", l.PID)
	err = proc.Signal(syscall.SIGHUP)
	if err != nil {
		debug.Log("signal error: %v, lock is probably stale\n", err)
		return false
	}
	return true
}
