//go:build aix || linux || solaris
// +build aix linux solaris

package signals

import (
	"os/signal"
	"syscall"
)

func setupSignals() {
	signal.Notify(signals.ch, syscall.SIGUSR1)
}
