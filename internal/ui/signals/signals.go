package signals

import (
	"os"
	"sync"
)

// GetProgressChannel returns a channel with which a single listener
// receives each incoming signal.
func GetProgressChannel() <-chan os.Signal {
	signals.Once.Do(func() {
		signals.ch = make(chan os.Signal, 1)
		setupSignals()
	})

	return signals.ch
}

// XXX The fact that signals is a single global variable means that only one
// listener receives each incoming signal.
var signals struct {
	ch chan os.Signal
	sync.Once
}
