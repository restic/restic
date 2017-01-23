// +build debug

package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"restic/errors"

	"github.com/pkg/profile"
)

var (
	listenMemoryProfile string
	memProfilePath      string
	cpuProfilePath      string

	prof interface {
		Stop()
	}
)

func init() {
	f := cmdRoot.PersistentFlags()
	f.StringVar(&listenMemoryProfile, "listen-profile", "", "listen on this `address:port` for memory profiling")
	f.StringVar(&memProfilePath, "mem-profile", "", "write memory profile to `dir`")
	f.StringVar(&cpuProfilePath, "cpu-profile", "", "write cpu profile to `dir`")
}

func runDebug() error {
	if listenMemoryProfile != "" {
		fmt.Fprintf(os.Stderr, "running memory profile HTTP server on %v\n", listenMemoryProfile)
		go func() {
			err := http.ListenAndServe(listenMemoryProfile, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "memory profile listen failed: %v\n", err)
			}
		}()
	}

	if memProfilePath != "" && cpuProfilePath != "" {
		return errors.Fatal("only one profile (memory or CPU) may be activated at the same time")
	}

	if memProfilePath != "" {
		prof = profile.Start(profile.Quiet, profile.MemProfile, profile.ProfilePath(memProfilePath))
	} else if memProfilePath != "" {
		prof = profile.Start(profile.Quiet, profile.CPUProfile, profile.ProfilePath(memProfilePath))
	}

	return nil
}

func shutdownDebug() {
	if prof != nil {
		prof.Stop()
	}
}
