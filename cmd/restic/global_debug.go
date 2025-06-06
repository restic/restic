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
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/pkg/profile"
)

func registerProfiling(cmd *cobra.Command) {
	var profiler profiler

	origPreRun := cmd.PersistentPreRunE
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if origPreRun != nil {
			if err := origPreRun(cmd, args); err != nil {
				return err
			}
		}
		return profiler.Start(profiler.opts)
	}

	// Once https://github.com/spf13/cobra/issues/1893 is fixed,
	// this could use PersistentPostRunE instead of OnFinalize,
	// reverting https://github.com/restic/restic/pull/5373.
	cobra.OnFinalize(func() {
		profiler.Stop()
	})

	profiler.opts.AddFlags(cmd.PersistentFlags())
}

type profiler struct {
	opts ProfileOptions
	stop interface {
		Stop()
	}
}

type ProfileOptions struct {
	listen    string
	memPath   string
	cpuPath   string
	tracePath string
	blockPath string
	insecure  bool
}

func (opts *ProfileOptions) AddFlags(f *pflag.FlagSet) {
	f.StringVar(&opts.listen, "listen-profile", "", "listen on this `address:port` for memory profiling")
	f.StringVar(&opts.memPath, "mem-profile", "", "write memory profile to `dir`")
	f.StringVar(&opts.cpuPath, "cpu-profile", "", "write cpu profile to `dir`")
	f.StringVar(&opts.tracePath, "trace-profile", "", "write trace to `dir`")
	f.StringVar(&opts.blockPath, "block-profile", "", "write block profile to `dir`")
	f.BoolVar(&opts.insecure, "insecure-kdf", false, "use insecure KDF settings")
}

type fakeTestingTB struct{}

func (fakeTestingTB) Logf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg, args...)
}

func (p *profiler) Start(profileOpts ProfileOptions) error {
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
		p.stop = profile.Start(profile.Quiet, profile.NoShutdownHook, profile.MemProfile, profile.ProfilePath(profileOpts.memPath))
	} else if profileOpts.cpuPath != "" {
		p.stop = profile.Start(profile.Quiet, profile.NoShutdownHook, profile.CPUProfile, profile.ProfilePath(profileOpts.cpuPath))
	} else if profileOpts.tracePath != "" {
		p.stop = profile.Start(profile.Quiet, profile.NoShutdownHook, profile.TraceProfile, profile.ProfilePath(profileOpts.tracePath))
	} else if profileOpts.blockPath != "" {
		p.stop = profile.Start(profile.Quiet, profile.NoShutdownHook, profile.BlockProfile, profile.ProfilePath(profileOpts.blockPath))
	}

	if profileOpts.insecure {
		repository.TestUseLowSecurityKDFParameters(fakeTestingTB{})
	}

	return nil
}

func (p *profiler) Stop() {
	if p.stop != nil {
		p.stop.Stop()
	}
}
