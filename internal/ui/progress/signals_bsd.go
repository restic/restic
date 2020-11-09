// +build darwin dragonfly freebsd netbsd openbsd

package progress

import (
	"os/signal"
	"syscall"
)

func setupSignals() {
	signal.Notify(signals.ch, syscall.SIGINFO, syscall.SIGUSR1)
}
