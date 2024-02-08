package main

import (
	"context"
	"fmt"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/spf13/cobra"
)

var cmdKeyRemove = &cobra.Command{
	Use:   "remove [ID]",
	Short: "Remove key ID (password) from the repository.",
	Long: `
The "remove" sub-command removes the selected key ID. The "remove" command does not allow
removing the current key being used to access the repository. 

EXIT STATUS
===========

Exit status is 0 if the command is successful, and non-zero if there was any error.
	`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runKeyRemove(cmd.Context(), globalOptions, args)
	},
}

func init() {
	cmdKey.AddCommand(cmdKeyRemove)
}

func runKeyRemove(ctx context.Context, gopts GlobalOptions, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("key remove expects one argument as the key id")
	}

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	lock, ctx, err := lockRepoExclusive(ctx, repo, gopts.RetryLock, gopts.JSON)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	idPrefix := args[0]

	return deleteKey(ctx, repo, idPrefix)
}

func deleteKey(ctx context.Context, repo *repository.Repository, idPrefix string) error {
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

	Verbosef("removed key %v\n", id)
	return nil
}
