package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/spf13/cobra"
)

var cmdKeyAdd = &cobra.Command{
	Use:   "add",
	Short: "Add a new key (password) to the repository; returns the new key ID",
	Long: `
The "add" sub-command creates a new key and validates the key. Returns the new key ID.

EXIT STATUS
===========

Exit status is 0 if the command is successful, and non-zero if there was any error.
	`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runKeyAdd(cmd.Context(), globalOptions, keyAddOpts, args)
	},
}

type KeyAddOptions struct {
	NewPasswordFile string
	Username        string
	Hostname        string
}

var keyAddOpts KeyAddOptions

func init() {
	cmdKey.AddCommand(cmdKeyAdd)

	flags := cmdKeyAdd.Flags()
	flags.StringVarP(&keyAddOpts.NewPasswordFile, "new-password-file", "", "", "`file` from which to read the new password")
	flags.StringVarP(&keyAddOpts.Username, "user", "", "", "the username for new key")
	flags.StringVarP(&keyAddOpts.Hostname, "host", "", "", "the hostname for new key")
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
	pw, err := getNewPassword(gopts, opts.NewPasswordFile)
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

func getNewPassword(gopts GlobalOptions, newPasswordFile string) (string, error) {
	if testKeyNewPassword != "" {
		return testKeyNewPassword, nil
	}

	if newPasswordFile != "" {
		return loadPasswordFromFile(newPasswordFile)
	}

	// Since we already have an open repository, temporary remove the password
	// to prompt the user for the passwd.
	newopts := gopts
	newopts.password = ""

	return ReadPasswordTwice(newopts,
		"enter new password: ",
		"enter password again: ")
}

func loadPasswordFromFile(pwdFile string) (string, error) {
	s, err := os.ReadFile(pwdFile)
	if os.IsNotExist(err) {
		return "", errors.Fatalf("%s does not exist", pwdFile)
	}
	return strings.TrimSpace(string(s)), errors.Wrap(err, "Readfile")
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
