package main

import (
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"

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
		return runInit(globalOptions, args)
	},
}

func init() {
	cmdRoot.AddCommand(cmdInit)
}

func runInit(gopts GlobalOptions, args []string) error {
	if gopts.Repo == "" {
		return errors.Fatal("Please specify repository location (-r)")
	}

	be, err := create(gopts.Repo, gopts.extended)
	if err != nil {
		return errors.Fatalf("create repository at %s failed: %v\n", gopts.Repo, err)
	}

	gopts.password, err = ReadPasswordTwice(gopts,
		"enter password for new repository: ",
		"enter password again: ")
	if err != nil {
		return err
	}

	s := repository.New(be, gopts.MinPackSize)

	err = s.Init(gopts.ctx, gopts.password)
	if err != nil {
		return errors.Fatalf("create key in repository at %s failed: %v\n", gopts.Repo, err)
	}

	Verbosef("created restic repository %v at %s\n", s.Config().ID[:10], gopts.Repo)
	Verbosef("\n")
	Verbosef("Please note that knowledge of your password is required to access\n")
	Verbosef("the repository. Losing your password means that your data is\n")
	Verbosef("irrecoverably lost.\n")

	return nil
}
