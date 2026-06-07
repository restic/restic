package repository

import (
	"os"

	"github.com/restic/restic/internal/debug"
)

// checkProcess will check if the process retaining the lock exists.
// Returns true if the process exists.
func (l *Lock) processExists() bool {
	proc, err := os.FindProcess(l.PID)
	if err != nil {
		debug.Log("error searching for process %d: %v\n", l.PID, err)
		return false
	}
	err = proc.Release()
	if err != nil {
		debug.Log("error releasing process %d: %v\n", l.PID, err)
	}
	return true
}
