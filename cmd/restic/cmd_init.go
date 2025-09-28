package main

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newInitCommand(globalOptions *global.Options) *cobra.Command {
	var opts InitOptions

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new repository",
		Long: `
The "init" command initializes a new repository.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
`,
		GroupID:           cmdGroupDefault,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd.Context(), opts, *globalOptions, args, globalOptions.Term)
		},
	}
	opts.AddFlags(cmd.Flags())
	return cmd
}

// InitOptions bundles all options for the init command.
type InitOptions struct {
	global.SecondaryRepoOptions
	CopyChunkerParameters bool
	RepositoryVersion     string
}

func (opts *InitOptions) AddFlags(f *pflag.FlagSet) {
	opts.SecondaryRepoOptions.AddFlags(f, "secondary", "to copy chunker parameters from")
	f.BoolVar(&opts.CopyChunkerParameters, "copy-chunker-params", false, "copy chunker parameters from the secondary repository (useful with the copy command)")
	f.StringVar(&opts.RepositoryVersion, "repository-version", "stable", "repository format version to use, allowed values are a format version, 'latest' and 'stable'")
}

func runInit(ctx context.Context, opts InitOptions, gopts global.Options, args []string, term ui.Terminal) error {
	if len(args) > 0 {
		return errors.Fatal("the init command expects no arguments, only options - please see `restic help init` for usage and flags")
	}

	printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, term)

	var version uint
	switch opts.RepositoryVersion {
	case "latest", "":
		version = restic.MaxRepoVersion
	case "stable":
		version = restic.StableRepoVersion
	default:
		v, err := strconv.ParseUint(opts.RepositoryVersion, 10, 32)
		if err != nil {
			return errors.Fatal("invalid repository version")
		}
		version = uint(v)
	}

	chunkerPolynomial, err := maybeReadChunkerPolynomial(ctx, opts, gopts, printer)
	if err != nil {
		return err
	}

	s, err := global.CreateRepository(ctx, gopts, version, chunkerPolynomial, printer)
	if err != nil {
		return errors.Fatalf("%s", err)
	}

	if !gopts.JSON {
		printer.P("created restic repository %v at %s", s.Config().ID[:10], location.StripPassword(gopts.Backends, gopts.Repo))
		if opts.CopyChunkerParameters && chunkerPolynomial != nil {
			printer.P(" with chunker parameters copied from secondary repository")
		}
		printer.P("")
		printer.P("Please note that knowledge of your password is required to access")
		printer.P("the repository. Losing your password means that your data is")
		printer.P("irrecoverably lost.")

	} else {
		status := initSuccess{
			MessageType: "initialized",
			ID:          s.Config().ID,
			Repository:  location.StripPassword(gopts.Backends, gopts.Repo),
		}
		return json.NewEncoder(gopts.Term.OutputWriter()).Encode(status)
	}

	return nil
}

func maybeReadChunkerPolynomial(ctx context.Context, opts InitOptions, gopts global.Options, printer progress.Printer) (*chunker.Pol, error) {
	if opts.CopyChunkerParameters {
		otherGopts, _, err := opts.SecondaryRepoOptions.FillGlobalOpts(ctx, gopts, "secondary")
		if err != nil {
			return nil, err
		}

		otherRepo, err := global.OpenRepository(ctx, otherGopts, printer)
		if err != nil {
			return nil, err
		}

		pol := otherRepo.Config().ChunkerPolynomial
		return &pol, nil
	}

	if opts.Repo != "" || opts.RepositoryFile != "" || opts.LegacyRepo != "" || opts.LegacyRepositoryFile != "" {
		return nil, errors.Fatal("Secondary repository must only be specified when copying the chunker parameters")
	}
	return nil, nil
}

type initSuccess struct {
	MessageType string `json:"message_type"` // "initialized"
	ID          string `json:"id"`
	Repository  string `json:"repository"`
}
