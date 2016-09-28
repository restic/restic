package main

import (
	"restic/errors"
	"restic/repository"

	"github.com/spf13/cobra"
)

var cmdInit = &cobra.Command{
	Use:   "init",
	Short: "initialize a new repository",
	Long: `
The "init" command initializes a new repository.
`,
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

	be, err := create(gopts.Repo)
	if err != nil {
		return errors.Fatalf("create backend at %s failed: %v\n", gopts.Repo, err)
	}

	if gopts.password == "" {
		gopts.password, err = ReadPasswordTwice(gopts,
			"enter password for new backend: ",
			"enter password again: ")
		if err != nil {
			return err
		}
	}

	s := repository.New(be)

	err = s.Init(gopts.password)
	if err != nil {
		return errors.Fatalf("create key in backend at %s failed: %v\n", gopts.Repo, err)
	}

	Verbosef("created restic backend %v at %s\n", s.Config().ID[:10], gopts.Repo)
	Verbosef("\n")
	Verbosef("Please note that knowledge of your password is required to access\n")
	Verbosef("the repository. Losing your password means that your data is\n")
	Verbosef("irrecoverably lost.\n")

	return nil
}
