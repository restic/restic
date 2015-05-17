package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/repository"
)

type CmdKey struct{}

func init() {
	_, err := parser.AddCommand("key",
		"manage keys",
		"The key command manages keys (passwords) of a repository",
		&CmdKey{})
	if err != nil {
		panic(err)
	}
}

func listKeys(s *repository.Repository) error {
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

func addKey(s *repository.Repository) error {
	pw := readPassword("RESTIC_NEWPASSWORD", "enter password for new key: ")
	pw2 := readPassword("RESTIC_NEWPASSWORD", "enter password again: ")

	if pw != pw2 {
		return errors.New("passwords do not match")
	}

	id, err := repository.AddKey(s, pw, s.Key())
	if err != nil {
		return fmt.Errorf("creating new key failed: %v\n", err)
	}

	fmt.Printf("saved new key as %s\n", id)

	return nil
}

func deleteKey(repo *repository.Repository, name string) error {
	if name == repo.KeyName() {
		return errors.New("refusing to remove key currently used to access repository")
	}

	err := repo.Remove(backend.Key, name)
	if err != nil {
		return err
	}

	fmt.Printf("removed key %v\n", name)
	return nil
}

func changePassword(s *repository.Repository) error {
	pw := readPassword("RESTIC_NEWPASSWORD", "enter password for new key: ")
	pw2 := readPassword("RESTIC_NEWPASSWORD", "enter password again: ")

	if pw != pw2 {
		return errors.New("passwords do not match")
	}

	// add new key
	id, err := repository.AddKey(s, pw, s.Key())
	if err != nil {
		return fmt.Errorf("creating new key failed: %v\n", err)
	}

	// remove old key
	err = s.Remove(backend.Key, s.KeyName())
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

	s, err := OpenRepo()
	if err != nil {
		return err
	}

	switch args[0] {
	case "list":
		return listKeys(s)
	case "add":
		return addKey(s)
	case "rm":
		id, err := backend.Find(s.Backend(), backend.Key, args[1])
		if err != nil {
			return err
		}

		return deleteKey(s, id)
	case "passwd":
		return changePassword(s)
	}

	return nil
}
