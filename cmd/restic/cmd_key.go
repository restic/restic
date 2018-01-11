package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/options"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/table"

	"github.com/spf13/cobra"
)

var cmdKey = &cobra.Command{
	Use:   "key [list|add|remove|passwd] [ID] [key-username=<username>] [key-hostname=<hostname>]",
	Short: "Manage keys (passwords)",
	Long: `
The "key" command manages keys (passwords) for accessing the repository.

By default keys are added for the current user and hostname, this can be
overridden by with the optional "key-username" and "key-hostname" parameters
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

func listKeys(ctx context.Context, s *repository.Repository, gopts GlobalOptions) error {
	type keyInfo struct {
		Current  bool   `json:"current"`
		ID       string `json:"id"`
		UserName string `json:"userName"`
		HostName string `json:"hostName"`
		Created  string `json:"created"`
	}

	var keys []keyInfo

	err := s.List(ctx, restic.KeyFile, func(id restic.ID, size int64) error {
		k, err := repository.LoadKey(ctx, s, id.String())
		if err != nil {
			Warnf("LoadKey() failed: %v\n", err)
			return nil
		}

		key := keyInfo{
			Current:  id.String() == s.KeyName(),
			ID:       id.Str(),
			UserName: k.Username,
			HostName: k.Hostname,
			Created:  k.Created.Local().Format(TimeFormat),
		}

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
		"enter password for new key: ",
		"enter password again: ")
}

func getKeyDetails(gopts GlobalOptions, args []string) (string, *string, *string, error) {
	var pw = ""
	var hostname = (*string)(nil)
	var username = (*string)(nil)

	opts, err := options.Parse(args)
	if err != nil {
		return "", nil, nil, err
	}

	pw, err = getNewPassword(gopts)
	if err != nil {
		return "", nil, nil, err
	}

	if host, set := opts["key-hostname"]; set {
		hostname = &host
	}

	if user, set := opts["key-username"]; set {
		username = &user
	}

	return pw, hostname, username, nil
}

func addKey(gopts GlobalOptions, repo *repository.Repository, args []string) error {

	pw, hostname, username, err := getKeyDetails(gopts, args)
	if err != nil {
		return err
	}

	id, err := repository.AddKey(gopts.ctx, repo, pw, hostname, username, repo.Key())
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

func changePassword(gopts GlobalOptions, repo *repository.Repository, args []string) error {

	pw, err := getNewPassword(gopts)
	if err != nil {
		return err
	}

	id, err := repository.AddKey(gopts.ctx, repo, pw, nil, nil, repo.Key())
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
	if len(args) < 1 || (args[0] == "remove" && len(args) != 2) ||
		(args[0] != "remove" && args[0] != "add" && len(args) != 1) ||
		(args[0] == "add" && len(args) > 4) {
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

		return listKeys(ctx, repo, gopts)
	case "add":
		lock, err := lockRepo(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}

		return addKey(gopts, repo, args[1:])
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

		return changePassword(gopts, repo, args[1:])
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
