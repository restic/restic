package main

import (
	"context"
	"fmt"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/spf13/cobra"
)

var cmdKeyPasswd = &cobra.Command{
	Use:   "passwd",
	Short: "Change key (password); creates a new key ID and removes the old key ID, returns new key ID",
	Long: `
The "passwd" sub-command creates a new key, validates the key and remove the old key ID.
Returns the new key ID. 

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
	`,
	DisableAutoGenTag: true,
}

type KeyPasswdOptions struct {
	KeyAddOptions
}

func init() {
	cmdKey.AddCommand(cmdKeyPasswd)

	var keyPasswdOpts KeyPasswdOptions
	keyPasswdOpts.KeyAddOptions.Add(cmdKeyPasswd.Flags())
	cmdKeyPasswd.RunE = func(cmd *cobra.Command, args []string) error {
		return runKeyPasswd(cmd.Context(), globalOptions, keyPasswdOpts, args)
	}
}

func runKeyPasswd(ctx context.Context, gopts GlobalOptions, opts KeyPasswdOptions, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("the key passwd command expects no arguments, only options - please see `restic help key passwd` for usage and flags")
	}

	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, false)
	if err != nil {
		return err
	}
	defer unlock()

	return changePassword(ctx, repo, gopts, opts)
}

func changePassword(ctx context.Context, repo *repository.Repository, gopts GlobalOptions, opts KeyPasswdOptions) error {
	pw, err := getNewPassword(ctx, gopts, opts.NewPasswordFile, opts.InsecureNoPassword)
	if err != nil {
		return err
	}

	id, err := repository.AddKey(ctx, repo, pw, "", "", repo.Key())
	if err != nil {
		return errors.Fatalf("creating new key failed: %v\n", err)
	}
	oldID := repo.KeyID()

	err = switchToNewKeyAndRemoveIfBroken(ctx, repo, id, pw)
	if err != nil {
		return err
	}

	err = repository.RemoveKey(ctx, repo, oldID)
	if err != nil {
		return err
	}

	Verbosef("saved new key as %s\n", id)

	return nil
}
