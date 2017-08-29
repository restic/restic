// +build debug

package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"

	"github.com/pkg/profile"
)

var (
	listenMemoryProfile string
	memProfilePath      string
	cpuProfilePath      string
	insecure            bool
)

func init() {
	f := cmdRoot.PersistentFlags()
	f.StringVar(&listenMemoryProfile, "listen-profile", "", "listen on this `address:port` for memory profiling")
	f.StringVar(&memProfilePath, "mem-profile", "", "write memory profile to `dir`")
	f.StringVar(&cpuProfilePath, "cpu-profile", "", "write cpu profile to `dir`")
	f.BoolVar(&insecure, "insecure-kdf", false, "use insecure KDF settings")
}

type fakeTestingTB struct{}

func (fakeTestingTB) Logf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg, args...)
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

	var prof interface {
		Stop()
	}

	if memProfilePath != "" {
		prof = profile.Start(profile.Quiet, profile.NoShutdownHook, profile.MemProfile, profile.ProfilePath(memProfilePath))
	} else if cpuProfilePath != "" {
		prof = profile.Start(profile.Quiet, profile.NoShutdownHook, profile.CPUProfile, profile.ProfilePath(cpuProfilePath))
	}

	if prof != nil {
		AddCleanupHandler(func() error {
			prof.Stop()
			return nil
		})
	}

	if insecure {
		repository.TestUseLowSecurityKDFParameters(fakeTestingTB{})
	}

	return nil
}
