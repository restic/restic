// +build linux solaris

package progress

import (
	"os/signal"
	"syscall"
)

func setupSignals() {
	signal.Notify(signals.ch, syscall.SIGUSR1)
}
