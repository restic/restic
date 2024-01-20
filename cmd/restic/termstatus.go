package main

import (
	"context"
	"sync"

	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/termstatus"
)

// setupTermstatus creates a new termstatus and reroutes globalOptions.{stdout,stderr} to it
// The returned function must be called to shut down the termstatus,
//
// Expected usage:
// ```
// term, cancel := setupTermstatus(ctx)
// defer cancel()
// // do stuff
// ```
func setupTermstatus(ctx context.Context) (*termstatus.Terminal, func()) {
	var wg sync.WaitGroup
	cancelCtx, cancel := context.WithCancel(ctx)

	term := termstatus.New(globalOptions.stdout, globalOptions.stderr, globalOptions.Quiet)
	wg.Add(1)
	go func() {
		defer wg.Done()
		term.Run(cancelCtx)
	}()

	// use the termstatus for stdout/stderr
	prevStdout, prevStderr := globalOptions.stdout, globalOptions.stderr
	stdioWrapper := ui.NewStdioWrapper(term)
	globalOptions.stdout, globalOptions.stderr = stdioWrapper.Stdout(), stdioWrapper.Stderr()

	return term, func() {
		// shutdown termstatus
		globalOptions.stdout, globalOptions.stderr = prevStdout, prevStderr
		cancel()
		wg.Wait()
	}
}
