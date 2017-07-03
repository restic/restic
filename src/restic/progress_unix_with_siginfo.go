// +build darwin

package restic

import (
	"os"
	"os/signal"
	"syscall"

	"restic/debug"
)

func init() {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGUSR1)
	signal.Notify(c, syscall.SIGINFO)
	go func() {
		for s := range c {
			debug.Log("Signal received: %v\n", s)
			forceUpdateProgress <- true
		}
	}()
}
