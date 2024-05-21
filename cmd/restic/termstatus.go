package main

import (
	"context"
	"sync"

	"github.com/restic/restic/internal/ui/termstatus"
)

// setupTermstatus creates a new termstatus and reroutes globalOptions.{stdout,stderr} to it
// The returned function must be called to shut down the termstatus,
//
// Expected usage:
// ```
// term, cancel := setupTermstatus()
// defer cancel()
// // do stuff
// ```
func setupTermstatus() (*termstatus.Terminal, func()) {
	var wg sync.WaitGroup
	// only shutdown once cancel is called to ensure that no output is lost
	cancelCtx, cancel := context.WithCancel(context.Background())

	term := termstatus.New(globalOptions.stdout, globalOptions.stderr, globalOptions.Quiet)
	wg.Add(1)
	go func() {
		defer wg.Done()
		term.Run(cancelCtx)
	}()

	// use the termstatus for stdout/stderr
	prevStdout, prevStderr := globalOptions.stdout, globalOptions.stderr
	globalOptions.stdout, globalOptions.stderr = termstatus.WrapStdio(term)

	return term, func() {
		// shutdown termstatus
		globalOptions.stdout, globalOptions.stderr = prevStdout, prevStderr
		cancel()
		wg.Wait()
	}
}
