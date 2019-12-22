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

	if gopts.MasterKeyFile == "" {
		gopts.password, err = ReadPasswordTwice(gopts,
			"enter password for new repository: ",
			"enter password again: ")
		if err != nil {
			return err
		}
	}

	s := repository.New(be)

	err = s.Init(gopts.ctx, gopts.MasterKeyFile, gopts.password)
	if err != nil {
		return errors.Fatalf("create key in repository at %s failed: %v\n", gopts.Repo, err)
	}

	Verbosef("created restic repository %v at %s\n", s.Config().ID[:10], gopts.Repo)
	Verbosef("\n")
	if gopts.MasterKeyFile == "" {
		Verbosef("Please note that knowledge of your password is required to access\n")
		Verbosef("the repository. Losing your password means that your data is\n")
		Verbosef("irrecoverably lost.\n")
	} else {
		Verbosef("Please note that you need the masterkey %s to access the repository\n", gopts.MasterKeyFile)
		Verbosef("Losing your masterkey file means that your data is irrecoverably lost.\n")
	}

	return nil
}
