// +build !windows,!darwin,!freebsd,!netbsd,!openbsd,!dragonfly,!solaris

package restic

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/restic/restic/internal/debug"
)

func init() {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGUSR1)
	go func() {
		for s := range c {
			debug.Log("Signal received: %v\n", s)
			forceUpdateProgress <- true
		}
	}()
}
