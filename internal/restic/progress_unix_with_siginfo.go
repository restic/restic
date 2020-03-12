// +build darwin freebsd netbsd openbsd dragonfly

package restic

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/restic/restic/internal/debug"
)

func progressSignalInit() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR1)
	signal.Notify(c, syscall.SIGINFO)
	go func() {
		for s := range c {
			debug.Log("Signal received: %v\n", s)
			forceUpdateProgress <- true
		}
	}()
}
