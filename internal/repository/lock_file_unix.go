//go:build !windows

package repository

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/restic/restic/internal/debug"
)

// listen for incoming SIGHUP and ignore
var ignoreSIGHUP sync.Once

func init() {
	ignoreSIGHUP.Do(func() {
		go func() {
			c := make(chan os.Signal, 1)
			signal.Notify(c, syscall.SIGHUP)
			for s := range c {
				debug.Log("Signal received: %v\n", s)
			}
		}()
	})
}

// checkProcess will check if the process retaining the lock
// exists and responds to SIGHUP signal.
// Returns true if the process exists and responds.
func (l *lockHandle) processExists() bool {
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
