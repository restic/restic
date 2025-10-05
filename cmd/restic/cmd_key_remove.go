package main

import (
	"context"
	"fmt"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/spf13/cobra"
)

func newKeyRemoveCommand(globalOptions *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove [ID]",
		Short: "Remove key ID (password) from the repository.",
		Long: `
The "remove" sub-command removes the selected key ID. The "remove" command does not allow
removing the current key being used to access the repository. 

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
	`,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeyRemove(cmd.Context(), *globalOptions, args, globalOptions.term)
		},
	}
	return cmd
}

func runKeyRemove(ctx context.Context, gopts GlobalOptions, args []string, term ui.Terminal) error {
	if len(args) != 1 {
		return fmt.Errorf("key remove expects one argument as the key id")
	}

	printer := ui.NewProgressPrinter(gopts.JSON, gopts.verbosity, term)
	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, false, printer)
	if err != nil {
		return err
	}
	defer unlock()

	return deleteKey(ctx, repo, args[0], printer)
}

func deleteKey(ctx context.Context, repo *repository.Repository, idPrefix string, printer progress.Printer) error {
	id, err := restic.Find(ctx, repo, restic.KeyFile, idPrefix)
	if err != nil {
		return err
	}

	if id == repo.KeyID() {
		return errors.Fatal("refusing to remove key currently used to access repository")
	}

	err = repository.RemoveKey(ctx, repo, id)
	if err != nil {
		return err
	}

	printer.P("removed key %v", id)
	return nil
}
