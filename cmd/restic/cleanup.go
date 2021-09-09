package main

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/restic/restic/internal/debug"
)

var cleanupHandlers struct {
	sync.Mutex
	list []func() error
	done bool
	ch   chan os.Signal
}

func init() {
	cleanupHandlers.ch = make(chan os.Signal, 1)
	go CleanupHandler(cleanupHandlers.ch)
	signal.Notify(cleanupHandlers.ch, syscall.SIGINT)
}

// AddCleanupHandler adds the function f to the list of cleanup handlers so
// that it is executed when all the cleanup handlers are run, e.g. when SIGINT
// is received.
func AddCleanupHandler(f func() error) {
	cleanupHandlers.Lock()
	defer cleanupHandlers.Unlock()

	// reset the done flag for integration tests
	cleanupHandlers.done = false

	cleanupHandlers.list = append(cleanupHandlers.list, f)
}

// RunCleanupHandlers runs all registered cleanup handlers
func RunCleanupHandlers() {
	cleanupHandlers.Lock()
	defer cleanupHandlers.Unlock()

	if cleanupHandlers.done {
		return
	}
	cleanupHandlers.done = true

	for _, f := range cleanupHandlers.list {
		err := f()
		if err != nil {
			Warnf("error in cleanup handler: %v\n", err)
		}
	}
	cleanupHandlers.list = nil
}

// CleanupHandler handles the SIGINT signals.
func CleanupHandler(c <-chan os.Signal) {
	for s := range c {
		debug.Log("signal %v received, cleaning up", s)
		Warnf("%ssignal %v received, cleaning up\n", clearLine(0), s)

		code := 0

		if s == syscall.SIGINT {
			code = 130
		} else {
			code = 1
		}

		Exit(code)
	}
}

// Exit runs the cleanup handlers and then terminates the process with the
// given exit code.
func Exit(code int) {
	RunCleanupHandlers()
	os.Exit(code)
}
