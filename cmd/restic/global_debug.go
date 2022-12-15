//go:build debug || profile
// +build debug profile

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
	listenProfile    string
	memProfilePath   string
	cpuProfilePath   string
	traceProfilePath string
	blockProfilePath string
	insecure         bool
)

func init() {
	f := cmdRoot.PersistentFlags()
	f.StringVar(&listenProfile, "listen-profile", "", "listen on this `address:port` for memory profiling")
	f.StringVar(&memProfilePath, "mem-profile", "", "write memory profile to `dir`")
	f.StringVar(&cpuProfilePath, "cpu-profile", "", "write cpu profile to `dir`")
	f.StringVar(&traceProfilePath, "trace-profile", "", "write trace to `dir`")
	f.StringVar(&blockProfilePath, "block-profile", "", "write block profile to `dir`")
	f.BoolVar(&insecure, "insecure-kdf", false, "use insecure KDF settings")
}

type fakeTestingTB struct{}

func (fakeTestingTB) Logf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg, args...)
}

func runDebug() error {
	if listenProfile != "" {
		fmt.Fprintf(os.Stderr, "running profile HTTP server on %v\n", listenProfile)
		go func() {
			err := http.ListenAndServe(listenProfile, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "profile HTTP server listen failed: %v\n", err)
			}
		}()
	}

	profilesEnabled := 0
	if memProfilePath != "" {
		profilesEnabled++
	}
	if cpuProfilePath != "" {
		profilesEnabled++
	}
	if traceProfilePath != "" {
		profilesEnabled++
	}
	if blockProfilePath != "" {
		profilesEnabled++
	}

	if profilesEnabled > 1 {
		return errors.Fatal("only one profile (memory, CPU, trace, or block) may be activated at the same time")
	}

	var prof interface {
		Stop()
	}

	if memProfilePath != "" {
		prof = profile.Start(profile.Quiet, profile.NoShutdownHook, profile.MemProfile, profile.ProfilePath(memProfilePath))
	} else if cpuProfilePath != "" {
		prof = profile.Start(profile.Quiet, profile.NoShutdownHook, profile.CPUProfile, profile.ProfilePath(cpuProfilePath))
	} else if traceProfilePath != "" {
		prof = profile.Start(profile.Quiet, profile.NoShutdownHook, profile.TraceProfile, profile.ProfilePath(traceProfilePath))
	} else if blockProfilePath != "" {
		prof = profile.Start(profile.Quiet, profile.NoShutdownHook, profile.BlockProfile, profile.ProfilePath(blockProfilePath))
	}

	if prof != nil {
		AddCleanupHandler(func(code int) (int, error) {
			prof.Stop()
			return code, nil
		})
	}

	if insecure {
		repository.TestUseLowSecurityKDFParameters(fakeTestingTB{})
	}

	return nil
}
