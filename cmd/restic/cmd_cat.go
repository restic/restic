package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
)

type CmdCat struct{}

func init() {
	_, err := parser.AddCommand("cat",
		"dump something",
		"The cat command dumps data structures or data from a repository",
		&CmdCat{})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdCat) Usage() string {
	return "[blob|tree|snapshot|key|lock] ID"
}

func (cmd CmdCat) Execute(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("type or ID not specified, Usage: %s", cmd.Usage())
	}

	s, err := OpenRepo()
	if err != nil {
		return err
	}

	tpe := args[0]

	id, err := backend.ParseID(args[1])
	if err != nil {
		id = nil

		if tpe != "snapshot" {
			return err
		}

		// find snapshot id with prefix
		id, err = s.FindSnapshot(args[1])
		if err != nil {
			return err
		}
	}

	switch tpe {
	case "blob":
		// try storage id
		data, err := s.LoadID(backend.Data, id)
		if err == nil {
			_, err = os.Stdout.Write(data)
			return err
		}

		_, err = os.Stdout.Write(data)
		return err

	case "tree":
		// try storage id
		tree := &restic.Tree{}
		err := s.LoadJSONID(backend.Tree, id, tree)
		if err != nil {
			return err
		}

		buf, err := json.MarshalIndent(&tree, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(buf))

		return nil
	case "snapshot":
		sn := &restic.Snapshot{}
		err = s.LoadJSONID(backend.Snapshot, id, sn)
		if err != nil {
			return err
		}

		buf, err := json.MarshalIndent(&sn, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(buf))

		return nil
	case "key":
		data, err := s.Get(backend.Key, id)
		if err != nil {
			return err
		}

		var key restic.Key
		err = json.Unmarshal(data, &key)
		if err != nil {
			return err
		}

		buf, err := json.MarshalIndent(&key, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(buf))

		return nil
	case "lock":
		return errors.New("not yet implemented")
	default:
		return errors.New("invalid type")
	}
}
