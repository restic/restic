package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/fd0/khepri"
	"github.com/fd0/khepri/backend"
)

func commandCat(be backend.Server, key *khepri.Key, args []string) error {
	if len(args) != 2 {
		return errors.New("usage: cat [blob|tree|snapshot|key|lock] ID")
	}

	tpe := args[0]

	id, err := backend.ParseID(args[1])
	if err != nil {
		return err
	}

	ch, err := khepri.NewContentHandler(be, key)
	if err != nil {
		return err
	}

	err = ch.LoadAllSnapshots()
	if err != nil {
		return err
	}

	switch tpe {
	case "blob":
		// try id
		data, err := ch.Load(backend.Blob, id)
		if err == nil {
			_, err = os.Stdout.Write(data)
			return err
		}

		// try storage id
		buf, err := be.Get(backend.Blob, id)
		if err != nil {
			return err
		}

		// decrypt
		buf, err = key.Decrypt(buf)
		if err != nil {
			return err
		}

		_, err = os.Stdout.Write(data)
		return err

	case "tree":
		var tree khepri.Tree
		// try id
		err := ch.LoadJSON(backend.Tree, id, &tree)
		if err != nil {
			// try storage id
			buf, err := be.Get(backend.Tree, id)
			if err != nil {
				return err
			}

			// decrypt
			buf, err = key.Decrypt(buf)
			if err != nil {
				return err
			}

			// unmarshal
			err = json.Unmarshal(backend.Uncompress(buf), &tree)
			if err != nil {
				return err
			}
		}

		buf, err := json.MarshalIndent(&tree, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(buf))

		return nil
	case "snapshot":
		var sn khepri.Snapshot
		err := ch.LoadJSONRaw(backend.Snapshot, id, &sn)
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
		data, err := be.Get(backend.Key, id)
		if err != nil {
			return err
		}

		var key khepri.Key
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

	return nil
}
