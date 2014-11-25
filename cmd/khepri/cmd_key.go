package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/fd0/khepri"
	"github.com/fd0/khepri/backend"
)

func list_keys(be backend.Server, key *khepri.Key) error {
	tab := NewTable()
	tab.Header = fmt.Sprintf("%-10s  %-10s  %-10s  %s", "ID", "User", "Host", "Created")
	tab.RowFormat = "%-10s  %-10s  %-10s  %s"

	plen, err := backend.PrefixLength(be, backend.Key)
	if err != nil {
		return err
	}

	backend.Each(be, backend.Key, func(id backend.ID, data []byte, err error) {
		k := khepri.Key{}
		err = json.Unmarshal(data, &k)
		if err != nil {
			return
		}

		tab.Rows = append(tab.Rows, []interface{}{id[:plen],
			k.Username, k.Hostname, k.Created.Format(TimeFormat)})
	})

	tab.Print(os.Stdout)

	return nil
}

func add_key(be backend.Server, key *khepri.Key) error {
	pw := readPassword("KHEPRI_NEWPASSWORD", "enter password for new key: ")
	pw2 := readPassword("KHEPRI_NEWPASSWORD", "enter password again: ")

	if pw != pw2 {
		errx(1, "passwords do not match")
	}

	id, err := key.AddKey(be, pw)
	if err != nil {
		return fmt.Errorf("creating new key failed: %v\n", err)
	}

	fmt.Printf("saved new key as %s\n", id)

	return nil
}

func delete_key(be backend.Server, key *khepri.Key, id backend.ID) error {
	if id.Equal(key.ID()) {
		return errors.New("refusing to remove key currently used to access repository")
	}

	err := be.Remove(backend.Key, id)
	if err != nil {
		return err
	}

	fmt.Printf("removed key %v\n", id)
	return nil
}

func commandKey(be backend.Server, key *khepri.Key, args []string) error {
	if len(args) < 1 || (args[0] == "rm" && len(args) != 2) {
		return errors.New("usage: key [list|add|rm] [ID]")
	}

	switch args[0] {
	case "list":
		return list_keys(be, key)
	case "add":
		return add_key(be, key)
	case "rm":
		id, err := backend.Find(be, backend.Key, args[1])
		if err != nil {
			return err
		}

		return delete_key(be, key, id)
	}

	return nil
}
