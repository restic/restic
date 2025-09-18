package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/restic/restic/internal/debug"
)

func createGlobalContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	ch := make(chan os.Signal, 1)
	go cleanupHandler(ch, cancel)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)

	return ctx
}

// cleanupHandler handles the SIGINT and SIGTERM signals.
func cleanupHandler(c <-chan os.Signal, cancel context.CancelFunc) {
	s := <-c
	debug.Log("signal %v received, cleaning up", s)
	// ignore error as there's no good way to handle it
	_, _ = fmt.Fprintf(os.Stderr, "\rsignal %v received, cleaning up \n", s)

	if val, _ := os.LookupEnv("RESTIC_DEBUG_STACKTRACE_SIGINT"); val != "" {
		_, _ = os.Stderr.WriteString("\n--- STACKTRACE START ---\n\n")
		_, _ = os.Stderr.WriteString(debug.DumpStacktrace())
		_, _ = os.Stderr.WriteString("\n--- STACKTRACE END ---\n")
	}

	cancel()
}

// Exit terminates the process with the given exit code.
func Exit(code int) {
	debug.Log("exiting with status code %d", code)
	os.Exit(code)
}
