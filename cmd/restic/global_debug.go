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

type ProfileOptions struct {
	listen    string
	memPath   string
	cpuPath   string
	tracePath string
	blockPath string
	insecure  bool
}

var profileOpts ProfileOptions
var prof interface {
	Stop()
}

func init() {
	f := cmdRoot.PersistentFlags()
	f.StringVar(&profileOpts.listen, "listen-profile", "", "listen on this `address:port` for memory profiling")
	f.StringVar(&profileOpts.memPath, "mem-profile", "", "write memory profile to `dir`")
	f.StringVar(&profileOpts.cpuPath, "cpu-profile", "", "write cpu profile to `dir`")
	f.StringVar(&profileOpts.tracePath, "trace-profile", "", "write trace to `dir`")
	f.StringVar(&profileOpts.blockPath, "block-profile", "", "write block profile to `dir`")
	f.BoolVar(&profileOpts.insecure, "insecure-kdf", false, "use insecure KDF settings")
}

type fakeTestingTB struct{}

func (fakeTestingTB) Logf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg, args...)
}

func runDebug() error {
	if profileOpts.listen != "" {
		fmt.Fprintf(os.Stderr, "running profile HTTP server on %v\n", profileOpts.listen)
		go func() {
			err := http.ListenAndServe(profileOpts.listen, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "profile HTTP server listen failed: %v\n", err)
			}
		}()
	}

	profilesEnabled := 0
	if profileOpts.memPath != "" {
		profilesEnabled++
	}
	if profileOpts.cpuPath != "" {
		profilesEnabled++
	}
	if profileOpts.tracePath != "" {
		profilesEnabled++
	}
	if profileOpts.blockPath != "" {
		profilesEnabled++
	}

	if profilesEnabled > 1 {
		return errors.Fatal("only one profile (memory, CPU, trace, or block) may be activated at the same time")
	}

	if profileOpts.memPath != "" {
		prof = profile.Start(profile.Quiet, profile.NoShutdownHook, profile.MemProfile, profile.ProfilePath(profileOpts.memPath))
	} else if profileOpts.cpuPath != "" {
		prof = profile.Start(profile.Quiet, profile.NoShutdownHook, profile.CPUProfile, profile.ProfilePath(profileOpts.cpuPath))
	} else if profileOpts.tracePath != "" {
		prof = profile.Start(profile.Quiet, profile.NoShutdownHook, profile.TraceProfile, profile.ProfilePath(profileOpts.tracePath))
	} else if profileOpts.blockPath != "" {
		prof = profile.Start(profile.Quiet, profile.NoShutdownHook, profile.BlockProfile, profile.ProfilePath(profileOpts.blockPath))
	}

	if profileOpts.insecure {
		repository.TestUseLowSecurityKDFParameters(fakeTestingTB{})
	}

	return nil
}

func stopDebug() {
	if prof != nil {
		prof.Stop()
	}
}
