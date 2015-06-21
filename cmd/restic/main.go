package main

import (
	"os"
	"runtime"

	"github.com/jessevdk/go-flags"
	"github.com/restic/restic/debug"
)

func init() {
	// set GOMAXPROCS to number of CPUs
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	// defer profile.Start(profile.MemProfileRate(100000), profile.ProfilePath(".")).Stop()
	// defer profile.Start(profile.CPUProfile, profile.ProfilePath(".")).Stop()
	globalOpts.Repo = os.Getenv("RESTIC_REPOSITORY")
	globalOpts.password = os.Getenv("RESTIC_PASSWORD")

	debug.Log("restic", "main %#v", os.Args)

	_, err := parser.Parse()
	if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
		os.Exit(0)
	}

	if err != nil {
		os.Exit(1)
	}
}
