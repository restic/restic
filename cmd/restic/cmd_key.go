package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
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

func list_keys(s restic.Server) error {
	tab := NewTable()
	tab.Header = fmt.Sprintf(" %-10s  %-10s  %-10s  %s", "ID", "User", "Host", "Created")
	tab.RowFormat = "%s%-10s  %-10s  %-10s  %s"

	plen, err := s.PrefixLength(backend.Key)
	if err != nil {
		return err
	}

	s.Each(backend.Key, func(id backend.ID, data []byte, err error) {
		k := restic.Key{}
		err = json.Unmarshal(data, &k)
		if err != nil {
			return
		}

		var current string
		if id.Equal(s.Key().ID()) {
			current = "*"
		} else {
			current = " "
		}
		tab.Rows = append(tab.Rows, []interface{}{current, id[:plen],
			k.Username, k.Hostname, k.Created.Format(TimeFormat)})
	})

	tab.Print(os.Stdout)

	return nil
}

func add_key(s restic.Server) error {
	pw := readPassword("RESTIC_NEWPASSWORD", "enter password for new key: ")
	pw2 := readPassword("RESTIC_NEWPASSWORD", "enter password again: ")

	if pw != pw2 {
		return errors.New("passwords do not match")
	}

	id, err := restic.AddKey(s, pw, s.Key())
	if err != nil {
		return fmt.Errorf("creating new key failed: %v\n", err)
	}

	fmt.Printf("saved new key as %s\n", id)

	return nil
}

func delete_key(s restic.Server, id backend.ID) error {
	if id.Equal(s.Key().ID()) {
		return errors.New("refusing to remove key currently used to access repository")
	}

	err := s.Remove(backend.Key, id)
	if err != nil {
		return err
	}

	fmt.Printf("removed key %v\n", id)
	return nil
}

func change_password(s restic.Server) error {
	pw := readPassword("RESTIC_NEWPASSWORD", "enter password for new key: ")
	pw2 := readPassword("RESTIC_NEWPASSWORD", "enter password again: ")

	if pw != pw2 {
		return errors.New("passwords do not match")
	}

	// add new key
	id, err := restic.AddKey(s, pw, s.Key())
	if err != nil {
		return fmt.Errorf("creating new key failed: %v\n", err)
	}

	// remove old key
	err = s.Remove(backend.Key, s.Key().ID())
	if err != nil {
		return err
	}

	fmt.Printf("saved new key as %s\n", id)

	return nil
}

func (cmd CmdKey) Usage() string {
	return "[list|add|rm|change] [ID]"
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
		return list_keys(s)
	case "add":
		return add_key(s)
	case "rm":
		id, err := backend.Find(s, backend.Key, args[1])
		if err != nil {
			return err
		}

		return delete_key(s, id)
	case "change":
		return change_password(s)
	}

	return nil
}
