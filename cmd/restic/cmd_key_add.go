package main

import (
	"context"
	"fmt"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var cmdKeyAdd = &cobra.Command{
	Use:   "add",
	Short: "Add a new key (password) to the repository; returns the new key ID",
	Long: `
The "add" sub-command creates a new key and validates the key. Returns the new key ID.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
	`,
	DisableAutoGenTag: true,
}

type KeyAddOptions struct {
	NewPasswordFile    string
	InsecureNoPassword bool
	Username           string
	Hostname           string
}

func (opts *KeyAddOptions) Add(flags *pflag.FlagSet) {
	flags.StringVarP(&opts.NewPasswordFile, "new-password-file", "", "", "`file` from which to read the new password")
	flags.BoolVar(&opts.InsecureNoPassword, "new-insecure-no-password", false, "add an empty password for the repository (insecure)")
	flags.StringVarP(&opts.Username, "user", "", "", "the username for new key")
	flags.StringVarP(&opts.Hostname, "host", "", "", "the hostname for new key")
}

func init() {
	cmdKey.AddCommand(cmdKeyAdd)

	var keyAddOpts KeyAddOptions
	keyAddOpts.Add(cmdKeyAdd.Flags())
	cmdKeyAdd.RunE = func(cmd *cobra.Command, args []string) error {
		return runKeyAdd(cmd.Context(), globalOptions, keyAddOpts, args)
	}
}

func runKeyAdd(ctx context.Context, gopts GlobalOptions, opts KeyAddOptions, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("the key add command expects no arguments, only options - please see `restic help key add` for usage and flags")
	}

	ctx, repo, unlock, err := openWithAppendLock(ctx, gopts, false)
	if err != nil {
		return err
	}
	defer unlock()

	return addKey(ctx, repo, gopts, opts)
}

func addKey(ctx context.Context, repo *repository.Repository, gopts GlobalOptions, opts KeyAddOptions) error {
	pw, err := getNewPassword(ctx, gopts, opts.NewPasswordFile, opts.InsecureNoPassword)
	if err != nil {
		return err
	}

	id, err := repository.AddKey(ctx, repo, pw, opts.Username, opts.Hostname, repo.Key())
	if err != nil {
		return errors.Fatalf("creating new key failed: %v\n", err)
	}

	err = switchToNewKeyAndRemoveIfBroken(ctx, repo, id, pw)
	if err != nil {
		return err
	}

	Verbosef("saved new key with ID %s\n", id.ID())

	return nil
}

// testKeyNewPassword is used to set a new password during integration testing.
var testKeyNewPassword string

func getNewPassword(ctx context.Context, gopts GlobalOptions, newPasswordFile string, insecureNoPassword bool) (string, error) {
	if testKeyNewPassword != "" {
		return testKeyNewPassword, nil
	}

	if insecureNoPassword {
		if newPasswordFile != "" {
			return "", fmt.Errorf("only either --new-password-file or --new-insecure-no-password may be specified")
		}
		return "", nil
	}

	if newPasswordFile != "" {
		password, err := loadPasswordFromFile(newPasswordFile)
		if err != nil {
			return "", err
		}
		if password == "" {
			return "", fmt.Errorf("an empty password is not allowed by default. Pass the flag `--new-insecure-no-password` to restic to disable this check")
		}
		return password, nil
	}

	// Since we already have an open repository, temporary remove the password
	// to prompt the user for the passwd.
	newopts := gopts
	newopts.password = ""
	// empty passwords are already handled above
	newopts.InsecureNoPassword = false

	return ReadPasswordTwice(ctx, newopts,
		"enter new password: ",
		"enter password again: ")
}

func switchToNewKeyAndRemoveIfBroken(ctx context.Context, repo *repository.Repository, key *repository.Key, pw string) error {
	// Verify new key to make sure it really works. A broken key can render the
	// whole repository inaccessible
	err := repo.SearchKey(ctx, pw, 0, key.ID().String())
	if err != nil {
		// the key is invalid, try to remove it
		_ = repository.RemoveKey(ctx, repo, key.ID())
		return errors.Fatalf("failed to access repository with new key: %v", err)
	}
	return nil
}
