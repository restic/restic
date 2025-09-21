package main

import (
	"context"
	"fmt"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newKeyPasswdCommand() *cobra.Command {
	var opts KeyPasswdOptions

	cmd := &cobra.Command{
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
Exit status is 12 if the password is incorrect.
	`,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			term, cancel := setupTermstatus()
			defer cancel()
			return runKeyPasswd(cmd.Context(), globalOptions, opts, args, term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

type KeyPasswdOptions struct {
	KeyAddOptions
}

func (opts *KeyPasswdOptions) AddFlags(flags *pflag.FlagSet) {
	opts.KeyAddOptions.Add(flags)
}

func runKeyPasswd(ctx context.Context, gopts GlobalOptions, opts KeyPasswdOptions, args []string, term ui.Terminal) error {
	if len(args) > 0 {
		return fmt.Errorf("the key passwd command expects no arguments, only options - please see `restic help key passwd` for usage and flags")
	}

	printer := newTerminalProgressPrinter(false, gopts.verbosity, term)
	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, false, printer)
	if err != nil {
		return err
	}
	defer unlock()

	return changePassword(ctx, repo, gopts, opts, printer)
}

func changePassword(ctx context.Context, repo *repository.Repository, gopts GlobalOptions, opts KeyPasswdOptions, printer progress.Printer) error {
	pw, err := getNewPassword(ctx, gopts, opts.NewPasswordFile, opts.InsecureNoPassword, printer)
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

	printer.P("saved new key as %s", id)

	return nil
}
