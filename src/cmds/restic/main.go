package main

import (
	"fmt"
	"os"
	"restic"
	"restic/debug"
	"runtime"

	"restic/errors"

	"github.com/jessevdk/go-flags"
)

func init() {
	// set GOMAXPROCS to number of CPUs
	if runtime.Version() < "go1.5" {
		gomaxprocs := os.Getenv("GOMAXPROCS")
		debug.Log("restic", "read GOMAXPROCS from env variable, value: %s", gomaxprocs)
		if gomaxprocs == "" {
			runtime.GOMAXPROCS(runtime.NumCPU())
		}
	}

}

func main() {
	// defer profile.Start(profile.MemProfileRate(100000), profile.ProfilePath(".")).Stop()
	// defer profile.Start(profile.CPUProfile, profile.ProfilePath(".")).Stop()
	globalOpts.Repo = os.Getenv("RESTIC_REPOSITORY")
	globalOpts.password = os.Getenv("RESTIC_PASSWORD")

	debug.Log("restic", "main %#v", os.Args)

	_, err := parser.Parse()
	if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
		parser.WriteHelp(os.Stdout)
		os.Exit(0)
	}

	debug.Log("main", "command returned error: %#v", err)

	switch {
	case restic.IsAlreadyLocked(errors.Cause(err)):
		fmt.Fprintf(os.Stderr, "%v\nthe `unlock` command can be used to remove stale locks\n", err)
	case errors.IsFatal(errors.Cause(err)):
		fmt.Fprintf(os.Stderr, "%v\n", err)
	case err != nil:
		fmt.Fprintf(os.Stderr, "%+v\n", err)
	}

	RunCleanupHandlers()

	if err != nil {
		os.Exit(1)
	}
}
