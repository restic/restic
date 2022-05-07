package main

import (
	"strconv"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/backend/location"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
)

var cmdInit = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new repository",
	Long: `
The "init" command initializes a new repository.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit(initOptions, globalOptions, args)
	},
}

// InitOptions bundles all options for the init command.
type InitOptions struct {
	secondaryRepoOptions
	CopyChunkerParameters bool
	RepositoryVersion     string
}

var initOptions InitOptions

func init() {
	cmdRoot.AddCommand(cmdInit)

	f := cmdInit.Flags()
	initSecondaryRepoOptions(f, &initOptions.secondaryRepoOptions, "secondary", "to copy chunker parameters from")
	f.BoolVar(&initOptions.CopyChunkerParameters, "copy-chunker-params", false, "copy chunker parameters from the secondary repository (useful with the copy command)")
	f.StringVar(&initOptions.RepositoryVersion, "repository-version", "stable", "repository format version to use, allowed values are a format version, 'latest' and 'stable'")
}

func runInit(opts InitOptions, gopts GlobalOptions, args []string) error {
	var version uint
	if opts.RepositoryVersion == "latest" || opts.RepositoryVersion == "" {
		version = restic.MaxRepoVersion
	} else if opts.RepositoryVersion == "stable" {
		version = restic.StableRepoVersion
	} else {
		v, err := strconv.ParseUint(opts.RepositoryVersion, 10, 32)
		if err != nil {
			return errors.Fatal("invalid repository version")
		}
		version = uint(v)
	}
	if version < restic.MinRepoVersion || version > restic.MaxRepoVersion {
		return errors.Fatalf("only repository versions between %v and %v are allowed", restic.MinRepoVersion, restic.MaxRepoVersion)
	}

	chunkerPolynomial, err := maybeReadChunkerPolynomial(opts, gopts)
	if err != nil {
		return err
	}

	repo, err := ReadRepo(gopts)
	if err != nil {
		return err
	}

	gopts.password, err = ReadPasswordTwice(gopts,
		"enter password for new repository: ",
		"enter password again: ")
	if err != nil {
		return err
	}

	be, err := create(repo, gopts.extended)
	if err != nil {
		return errors.Fatalf("create repository at %s failed: %v\n", location.StripPassword(gopts.Repo), err)
	}

	s, err := repository.New(be, repository.Options{
		Compression: gopts.Compression,
		PackSize:    gopts.PackSize * 1024 * 1024,
	})
	if err != nil {
		return err
	}

	err = s.Init(gopts.ctx, version, gopts.password, chunkerPolynomial)
	if err != nil {
		return errors.Fatalf("create key in repository at %s failed: %v\n", location.StripPassword(gopts.Repo), err)
	}

	Verbosef("created restic repository %v at %s\n", s.Config().ID[:10], location.StripPassword(gopts.Repo))
	Verbosef("\n")
	Verbosef("Please note that knowledge of your password is required to access\n")
	Verbosef("the repository. Losing your password means that your data is\n")
	Verbosef("irrecoverably lost.\n")

	return nil
}

func maybeReadChunkerPolynomial(opts InitOptions, gopts GlobalOptions) (*chunker.Pol, error) {
	if opts.CopyChunkerParameters {
		otherGopts, _, err := fillSecondaryGlobalOpts(opts.secondaryRepoOptions, gopts, "secondary")
		if err != nil {
			return nil, err
		}

		otherRepo, err := OpenRepository(otherGopts)
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
