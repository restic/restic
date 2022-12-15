//go:build darwin || dragonfly || freebsd || netbsd || openbsd
// +build darwin dragonfly freebsd netbsd openbsd

package signals

import (
	"os/signal"
	"syscall"
)

func setupSignals() {
	signal.Notify(signals.ch, syscall.SIGINFO, syscall.SIGUSR1)
}
