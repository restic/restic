package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
)

var cmdKey = &cobra.Command{
	Use:   "key [list|add|remove|passwd] [ID]",
	Short: "Manage keys (passwords)",
	Long: `
The "key" command manages keys (passwords) for accessing the repository.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runKey(globalOptions, args)
	},
}

var newPasswordFile string

func init() {
	cmdRoot.AddCommand(cmdKey)

	flags := cmdKey.Flags()
	flags.StringVarP(&newPasswordFile, "new-password-file", "", "", "the file from which to load a new password")
}

func listKeys(ctx context.Context, s *repository.Repository) error {
	tab := NewTable()
	tab.Header = fmt.Sprintf(" %-10s  %-10s  %-10s  %s", "ID", "User", "Host", "Created")
	tab.RowFormat = "%s%-10s  %-10s  %-10s  %s"

	err := s.List(ctx, restic.KeyFile, func(id restic.ID, size int64) error {
		k, err := repository.LoadKey(ctx, s, id.String())
		if err != nil {
			Warnf("LoadKey() failed: %v\n", err)
			return nil
		}

		var current string
		if id.String() == s.KeyName() {
			current = "*"
		} else {
			current = " "
		}
		tab.Rows = append(tab.Rows, []interface{}{current, id.Str(),
			k.Username, k.Hostname, k.Created.Format(TimeFormat)})
		return nil
	})
	if err != nil {
		return err
	}

	return tab.Write(globalOptions.stdout)
}

// testKeyNewPassword is used to set a new password during integration testing.
var testKeyNewPassword string

func getNewPassword(gopts GlobalOptions) (string, error) {
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
		"enter password for new key: ",
		"enter password again: ")
}

func addKey(gopts GlobalOptions, repo *repository.Repository) error {
	pw, err := getNewPassword(gopts)
	if err != nil {
		return err
	}

	id, err := repository.AddKey(gopts.ctx, repo, pw, repo.Key())
	if err != nil {
		return errors.Fatalf("creating new key failed: %v\n", err)
	}

	Verbosef("saved new key as %s\n", id)

	return nil
}

func deleteKey(ctx context.Context, repo *repository.Repository, name string) error {
	if name == repo.KeyName() {
		return errors.Fatal("refusing to remove key currently used to access repository")
	}

	h := restic.Handle{Type: restic.KeyFile, Name: name}
	err := repo.Backend().Remove(ctx, h)
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

	id, err := repository.AddKey(gopts.ctx, repo, pw, repo.Key())
	if err != nil {
		return errors.Fatalf("creating new key failed: %v\n", err)
	}

	h := restic.Handle{Type: restic.KeyFile, Name: repo.KeyName()}
	err = repo.Backend().Remove(gopts.ctx, h)
	if err != nil {
		return err
	}

	Verbosef("saved new key as %s\n", id)

	return nil
}

func runKey(gopts GlobalOptions, args []string) error {
	if len(args) < 1 || (args[0] == "remove" && len(args) != 2) || (args[0] != "remove" && len(args) != 1) {
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
	case "remove":
		lock, err := lockRepoExclusive(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}

		id, err := restic.Find(repo.Backend(), restic.KeyFile, args[1])
		if err != nil {
			return err
		}

		return deleteKey(gopts.ctx, repo, id)
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

func loadPasswordFromFile(pwdFile string) (string, error) {
	s, err := ioutil.ReadFile(pwdFile)
	if os.IsNotExist(err) {
		return "", errors.Fatalf("%s does not exist", pwdFile)
	}
	return strings.TrimSpace(string(s)), errors.Wrap(err, "Readfile")
}
