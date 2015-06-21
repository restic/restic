package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/repository"
)

type CmdKey struct {
	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("key",
		"manage keys",
		"The key command manages keys (passwords) of a repository",
		&CmdKey{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdKey) listKeys(s *repository.Repository) error {
	tab := NewTable()
	tab.Header = fmt.Sprintf(" %-10s  %-10s  %-10s  %s", "ID", "User", "Host", "Created")
	tab.RowFormat = "%s%-10s  %-10s  %-10s  %s"

	plen, err := s.PrefixLength(backend.Key)
	if err != nil {
		return err
	}

	done := make(chan struct{})
	defer close(done)

	for id := range s.List(backend.Key, done) {
		k, err := repository.LoadKey(s, id.String())
		if err != nil {
			fmt.Fprintf(os.Stderr, "LoadKey() failed: %v\n", err)
			continue
		}

		var current string
		if id.String() == s.KeyName() {
			current = "*"
		} else {
			current = " "
		}
		tab.Rows = append(tab.Rows, []interface{}{current, id.String()[:plen],
			k.Username, k.Hostname, k.Created.Format(TimeFormat)})
	}

	tab.Write(os.Stdout)

	return nil
}

func (cmd CmdKey) getNewPassword() (string, error) {
	newPassword := os.Getenv("RESTIC_NEWPASSWORD")

	if newPassword == "" {
		newPassword = cmd.global.ReadPasswordTwice(
			"enter password for new key: ",
			"enter password again: ")
	}

	return newPassword, nil
}

func (cmd CmdKey) addKey(repo *repository.Repository) error {
	newPassword, err := cmd.getNewPassword()
	if err != nil {
		return err
	}

	id, err := repository.AddKey(repo, newPassword, repo.Key())
	if err != nil {
		return fmt.Errorf("creating new key failed: %v\n", err)
	}

	fmt.Printf("saved new key as %s\n", id)

	return nil
}

func (cmd CmdKey) deleteKey(repo *repository.Repository, name string) error {
	if name == repo.KeyName() {
		return errors.New("refusing to remove key currently used to access repository")
	}

	err := repo.Backend().Remove(backend.Key, name)
	if err != nil {
		return err
	}

	fmt.Printf("removed key %v\n", name)
	return nil
}

func (cmd CmdKey) changePassword(repo *repository.Repository) error {
	newPassword, err := cmd.getNewPassword()
	if err != nil {
		return err
	}

	id, err := repository.AddKey(repo, newPassword, repo.Key())
	if err != nil {
		return fmt.Errorf("creating new key failed: %v\n", err)
	}

	err = repo.Backend().Remove(backend.Key, repo.KeyName())
	if err != nil {
		return err
	}

	fmt.Printf("saved new key as %s\n", id)

	return nil
}

func (cmd CmdKey) Usage() string {
	return "[list|add|rm|passwd] [ID]"
}

func (cmd CmdKey) Execute(args []string) error {
	if len(args) < 1 || (args[0] == "rm" && len(args) != 2) {
		return fmt.Errorf("wrong number of arguments, Usage: %s", cmd.Usage())
	}

	s, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	switch args[0] {
	case "list":
		return cmd.listKeys(s)
	case "add":
		return cmd.addKey(s)
	case "rm":
		id, err := backend.Find(s.Backend(), backend.Key, args[1])
		if err != nil {
			return err
		}

		return cmd.deleteKey(s, id)
	case "passwd":
		return cmd.changePassword(s)
	}

	return nil
}
