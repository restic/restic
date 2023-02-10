package restic

import (
	"os"
	"os/user"

	"github.com/restic/restic/internal/debug"
)

// uidGidInt always returns 0 on Windows, since uid isn't numbers
func uidGidInt(u *user.User) (uid, gid uint32, err error) {
	return 0, 0, nil
}

// checkProcess will check if the process retaining the lock exists.
// Returns true if the process exists.
func (l Lock) processExists() bool {
	proc, err := os.FindProcess(l.PID)
	if err != nil {
		debug.Log("error searching for process %d: %v\n", l.PID, err)
		return false
	}
	proc.Release()
	return true
}
