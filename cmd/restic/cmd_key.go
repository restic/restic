package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/table"

	"github.com/spf13/cobra"
)

var cmdKey = &cobra.Command{
	Use:   "key [flags] [list|add|remove|passwd] [ID]",
	Short: "Manage keys (passwords)",
	Long: `
The "key" command manages keys (passwords) for accessing the repository.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runKey(cmd.Context(), globalOptions, args)
	},
}

var (
	newPasswordFile string
	keyUsername     string
	keyHostname     string
)

func init() {
	cmdRoot.AddCommand(cmdKey)

	flags := cmdKey.Flags()
	flags.StringVarP(&newPasswordFile, "new-password-file", "", "", "`file` from which to read the new password")
	flags.StringVarP(&keyUsername, "user", "", "", "the username for new keys")
	flags.StringVarP(&keyHostname, "host", "", "", "the hostname for new keys")
}

func listKeys(ctx context.Context, s *repository.Repository, gopts GlobalOptions) error {
	type keyInfo struct {
		Current  bool   `json:"current"`
		ID       string `json:"id"`
		UserName string `json:"userName"`
		HostName string `json:"hostName"`
		Created  string `json:"created"`
	}

	var m sync.Mutex
	var keys []keyInfo

	err := restic.ParallelList(ctx, s.Backend(), restic.KeyFile, s.Connections(), func(ctx context.Context, id restic.ID, size int64) error {
		k, err := repository.LoadKey(ctx, s, id)
		if err != nil {
			Warnf("LoadKey() failed: %v\n", err)
			return nil
		}

		key := keyInfo{
			Current:  id == s.KeyID(),
			ID:       id.Str(),
			UserName: k.Username,
			HostName: k.Hostname,
			Created:  k.Created.Local().Format(TimeFormat),
		}

		m.Lock()
		defer m.Unlock()
		keys = append(keys, key)
		return nil
	})

	if err != nil {
		return err
	}

	if gopts.JSON {
		return json.NewEncoder(globalOptions.stdout).Encode(keys)
	}

	tab := table.New()
	tab.AddColumn(" ID", "{{if .Current}}*{{else}} {{end}}{{ .ID }}")
	tab.AddColumn("User", "{{ .UserName }}")
	tab.AddColumn("Host", "{{ .HostName }}")
	tab.AddColumn("Created", "{{ .Created }}")

	for _, key := range keys {
		tab.AddRow(key)
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
		"enter new password: ",
		"enter password again: ")
}

func addKey(ctx context.Context, repo *repository.Repository, gopts GlobalOptions) error {
	pw, err := getNewPassword(gopts)
	if err != nil {
		return err
	}

	id, err := repository.AddKey(ctx, repo, pw, keyUsername, keyHostname, repo.Key())
	if err != nil {
		return errors.Fatalf("creating new key failed: %v\n", err)
	}

	err = switchToNewKeyAndRemoveIfBroken(ctx, repo, id, pw)
	if err != nil {
		return err
	}

	Verbosef("saved new key as %s\n", id)

	return nil
}

func deleteKey(ctx context.Context, repo *repository.Repository, id restic.ID) error {
	if id == repo.KeyID() {
		return errors.Fatal("refusing to remove key currently used to access repository")
	}

	h := restic.Handle{Type: restic.KeyFile, Name: id.String()}
	err := repo.Backend().Remove(ctx, h)
	if err != nil {
		return err
	}

	Verbosef("removed key %v\n", id)
	return nil
}

func changePassword(ctx context.Context, repo *repository.Repository, gopts GlobalOptions) error {
	pw, err := getNewPassword(gopts)
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

	h := restic.Handle{Type: restic.KeyFile, Name: oldID.String()}
	err = repo.Backend().Remove(ctx, h)
	if err != nil {
		return err
	}

	Verbosef("saved new key as %s\n", id)

	return nil
}

func switchToNewKeyAndRemoveIfBroken(ctx context.Context, repo *repository.Repository, key *repository.Key, pw string) error {
	// Verify new key to make sure it really works. A broken key can render the
	// whole repository inaccessible
	err := repo.SearchKey(ctx, pw, 0, key.ID().String())
	if err != nil {
		// the key is invalid, try to remove it
		h := restic.Handle{Type: restic.KeyFile, Name: key.ID().String()}
		_ = repo.Backend().Remove(ctx, h)
		return errors.Fatalf("failed to access repository with new key: %v", err)
	}
	return nil
}

func runKey(ctx context.Context, gopts GlobalOptions, args []string) error {
	if len(args) < 1 || (args[0] == "remove" && len(args) != 2) || (args[0] != "remove" && len(args) != 1) {
		return errors.Fatal("wrong number of arguments")
	}

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	switch args[0] {
	case "list":
		lock, ctx, err := lockRepo(ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}

		return listKeys(ctx, repo, gopts)
	case "add":
		lock, ctx, err := lockRepo(ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}

		return addKey(ctx, repo, gopts)
	case "remove":
		lock, ctx, err := lockRepoExclusive(ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}

		id, err := restic.Find(ctx, repo.Backend(), restic.KeyFile, args[1])
		if err != nil {
			return err
		}

		return deleteKey(ctx, repo, id)
	case "passwd":
		lock, ctx, err := lockRepoExclusive(ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}

		return changePassword(ctx, repo, gopts)
	}

	return nil
}

func loadPasswordFromFile(pwdFile string) (string, error) {
	s, err := os.ReadFile(pwdFile)
	if os.IsNotExist(err) {
		return "", errors.Fatalf("%s does not exist", pwdFile)
	}
	return strings.TrimSpace(string(s)), errors.Wrap(err, "Readfile")
}
