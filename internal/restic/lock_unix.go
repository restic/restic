// +build !windows

package restic

import (
	"os"
	"os/user"
	"strconv"
	"syscall"

	"github.com/restic/restic/internal/errors"

	"github.com/restic/restic/internal/debug"
)

// uidGidInt returns uid, gid of the user as a number.
func uidGidInt(u user.User) (uid, gid uint32, err error) {
	var ui, gi int64
	ui, err = strconv.ParseInt(u.Uid, 10, 32)
	if err != nil {
		return uid, gid, errors.Wrap(err, "ParseInt")
	}
	gi, err = strconv.ParseInt(u.Gid, 10, 32)
	if err != nil {
		return uid, gid, errors.Wrap(err, "ParseInt")
	}
	uid = uint32(ui)
	gid = uint32(gi)
	return
}

// checkProcess will check if the process retaining the lock
// exists and responds to SIGHUP signal.
// Returns true if the process exists and responds.
func (l Lock) processExists() bool {
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
