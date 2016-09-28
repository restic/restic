package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"restic/debug"
)

var cleanupHandlers struct {
	sync.Mutex
	list []func() error
	done bool
}

var stderr = os.Stderr

func init() {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT)

	go CleanupHandler(c)
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
			fmt.Fprintf(stderr, "error in cleanup handler: %v\n", err)
		}
	}
	cleanupHandlers.list = nil
}

// CleanupHandler handles the SIGINT signal.
func CleanupHandler(c <-chan os.Signal) {
	for s := range c {
		debug.Log("signal %v received, cleaning up", s)
		fmt.Printf("%sInterrupt received, cleaning up\n", ClearLine())
		RunCleanupHandlers()
		fmt.Println("exiting")
		os.Exit(0)
	}
}
