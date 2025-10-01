package main

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newInitCommand() *cobra.Command {
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
			term, cancel := setupTermstatus()
			defer cancel()
			return runInit(cmd.Context(), opts, globalOptions, args, term)
		},
	}
	opts.AddFlags(cmd.Flags())
	return cmd
}

// InitOptions bundles all options for the init command.
type InitOptions struct {
	secondaryRepoOptions
	CopyChunkerParameters bool
	RepositoryVersion     string
}

func (opts *InitOptions) AddFlags(f *pflag.FlagSet) {
	opts.secondaryRepoOptions.AddFlags(f, "secondary", "to copy chunker parameters from")
	f.BoolVar(&opts.CopyChunkerParameters, "copy-chunker-params", false, "copy chunker parameters from the secondary repository (useful with the copy command)")
	f.StringVar(&opts.RepositoryVersion, "repository-version", "stable", "repository format version to use, allowed values are a format version, 'latest' and 'stable'")
}

func runInit(ctx context.Context, opts InitOptions, gopts GlobalOptions, args []string, term ui.Terminal) error {
	if len(args) > 0 {
		return errors.Fatal("the init command expects no arguments, only options - please see `restic help init` for usage and flags")
	}

	printer := newTerminalProgressPrinter(gopts.JSON, gopts.verbosity, term)

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

	if version < restic.MinRepoVersion || version > restic.MaxRepoVersion {
		return errors.Fatalf("only repository versions between %v and %v are allowed", restic.MinRepoVersion, restic.MaxRepoVersion)
	}

	chunkerPolynomial, err := maybeReadChunkerPolynomial(ctx, opts, gopts, printer)
	if err != nil {
		return err
	}

	gopts.Repo, err = ReadRepo(gopts)
	if err != nil {
		return err
	}

	gopts.password, err = ReadPasswordTwice(ctx, gopts,
		"enter password for new repository: ",
		"enter password again: ",
		printer)
	if err != nil {
		return err
	}

	be, err := create(ctx, gopts.Repo, gopts, gopts.extended, printer)
	if err != nil {
		return errors.Fatalf("create repository at %s failed: %v", location.StripPassword(gopts.backends, gopts.Repo), err)
	}

	s, err := repository.New(be, repository.Options{
		Compression: gopts.Compression,
		PackSize:    gopts.PackSize * 1024 * 1024,
	})
	if err != nil {
		return errors.Fatalf("%s", err)
	}

	err = s.Init(ctx, version, gopts.password, chunkerPolynomial)
	if err != nil {
		return errors.Fatalf("create key in repository at %s failed: %v", location.StripPassword(gopts.backends, gopts.Repo), err)
	}

	if !gopts.JSON {
		printer.P("created restic repository %v at %s", s.Config().ID[:10], location.StripPassword(gopts.backends, gopts.Repo))
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
			Repository:  location.StripPassword(gopts.backends, gopts.Repo),
		}
		return json.NewEncoder(globalOptions.stdout).Encode(status)
	}

	return nil
}

func maybeReadChunkerPolynomial(ctx context.Context, opts InitOptions, gopts GlobalOptions, printer progress.Printer) (*chunker.Pol, error) {
	if opts.CopyChunkerParameters {
		otherGopts, _, err := fillSecondaryGlobalOpts(ctx, opts.secondaryRepoOptions, gopts, "secondary", printer)
		if err != nil {
			return nil, err
		}

		otherRepo, err := OpenRepository(ctx, otherGopts, printer)
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
