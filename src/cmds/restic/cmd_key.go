package main

import (
	"context"
	"fmt"
	"restic"
	"restic/errors"
	"restic/repository"

	"github.com/spf13/cobra"
)

var cmdKey = &cobra.Command{
	Use:   "key [list|add|rm|passwd] [ID]",
	Short: "manage keys (passwords)",
	Long: `
The "key" command manages keys (passwords) for accessing the repository.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runKey(globalOptions, args)
	},
}

func init() {
	cmdRoot.AddCommand(cmdKey)
}

func listKeys(ctx context.Context, s *repository.Repository) error {
	tab := NewTable()
	tab.Header = fmt.Sprintf(" %-10s  %-10s  %-10s  %s", "ID", "User", "Host", "Created")
	tab.RowFormat = "%s%-10s  %-10s  %-10s  %s"

	for id := range s.List(restic.KeyFile, ctx.Done()) {
		k, err := repository.LoadKey(s.Backend(), id.String())
		if err != nil {
			Warnf("LoadKey() failed: %v\n", err)
			continue
		}

		var current string
		if id.String() == s.KeyName() {
			current = "*"
		} else {
			current = " "
		}
		tab.Rows = append(tab.Rows, []interface{}{current, id.Str(),
			k.Username, k.Hostname, k.Created.Format(TimeFormat)})
	}

	return tab.Write(globalOptions.stdout)
}

// testKeyNewPassword is used to set a new password during integration testing.
var testKeyNewPassword string

func getNewPassword(gopts GlobalOptions) (string, error) {
	if testKeyNewPassword != "" {
		return testKeyNewPassword, nil
	}

	return ReadPasswordTwice(gopts,
		"enter password for new key: ",
		"enter password again: ")
}

func addKey(gopts GlobalOptions, repo *repository.Repository) error {
	pw, err := getNewPassword(gopts)
	if err != nil {
		return err
	}

	id, err := repository.AddKey(repo.Backend(), pw, repo.Key())
	if err != nil {
		return errors.Fatalf("creating new key failed: %v\n", err)
	}

	Verbosef("saved new key as %s\n", id)

	return nil
}

func deleteKey(repo *repository.Repository, name string) error {
	if name == repo.KeyName() {
		return errors.Fatal("refusing to remove key currently used to access repository")
	}

	h := restic.Handle{Type: restic.KeyFile, Name: name}
	err := repo.Backend().Remove(h)
	if err != nil {
		return err
	}

	Verbosef("removed key %v\n", name)
	return nil
}

func changePassword(gopts GlobalOptions, repo *repository.Repository) error {
	pw, err := getNewPassword(gopts)
	if err != nil {
		return err
	}

	id, err := repository.AddKey(repo.Backend(), pw, repo.Key())
	if err != nil {
		return errors.Fatalf("creating new key failed: %v\n", err)
	}

	h := restic.Handle{Type: restic.KeyFile, Name: repo.KeyName()}
	err = repo.Backend().Remove(h)
	if err != nil {
		return err
	}

	Verbosef("saved new key as %s\n", id)

	return nil
}

func runKey(gopts GlobalOptions, args []string) error {
	if len(args) < 1 || (args[0] == "rm" && len(args) != 2) || (args[0] != "rm" && len(args) != 1) {
		return errors.Fatal("wrong number of arguments")
	}

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	switch args[0] {
	case "list":
		lock, err := lockRepo(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}

		return listKeys(ctx, repo)
	case "add":
		lock, err := lockRepo(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}

		return addKey(gopts, repo)
	case "rm":
		lock, err := lockRepoExclusive(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}

		id, err := restic.Find(repo.Backend(), restic.KeyFile, args[1])
		if err != nil {
			return err
		}

		return deleteKey(repo, id)
	case "passwd":
		lock, err := lockRepoExclusive(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}

		return changePassword(gopts, repo)
	}

	return nil
}
