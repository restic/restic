package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"restic"
	"restic/backend"
	"restic/debug"
	"restic/pack"
	"restic/repository"
)

type CmdCat struct {
	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("cat",
		"dump something",
		"The cat command dumps data structures or data from a repository",
		&CmdCat{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdCat) Usage() string {
	return "[pack|blob|tree|snapshot|key|masterkey|config|lock] ID"
}

func (cmd CmdCat) Execute(args []string) error {
	if len(args) < 1 || (args[0] != "masterkey" && args[0] != "config" && len(args) != 2) {
		return fmt.Errorf("type or ID not specified, Usage: %s", cmd.Usage())
	}

	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}

	lock, err := lockRepo(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	tpe := args[0]

	var id backend.ID
	if tpe != "masterkey" && tpe != "config" {
		id, err = backend.ParseID(args[1])
		if err != nil {
			if tpe != "snapshot" {
				return err
			}

			// find snapshot id with prefix
			id, err = restic.FindSnapshot(repo, args[1])
			if err != nil {
				return err
			}
		}
	}

	// handle all types that don't need an index
	switch tpe {
	case "config":
		buf, err := json.MarshalIndent(repo.Config, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(buf))
		return nil
	case "index":
		buf, err := repo.LoadAndDecrypt(backend.Index, id)
		if err != nil {
			return err
		}

		_, err = os.Stdout.Write(append(buf, '\n'))
		return err

	case "snapshot":
		sn := &restic.Snapshot{}
		err = repo.LoadJSONUnpacked(backend.Snapshot, id, sn)
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
		h := backend.Handle{Type: backend.Key, Name: id.String()}
		buf, err := backend.LoadAll(repo.Backend(), h, nil)
		if err != nil {
			return err
		}

		key := &repository.Key{}
		err = json.Unmarshal(buf, key)
		if err != nil {
			return err
		}

		buf, err = json.MarshalIndent(&key, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(buf))
		return nil
	case "masterkey":
		buf, err := json.MarshalIndent(repo.Key(), "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(buf))
		return nil
	case "lock":
		lock, err := restic.LoadLock(repo, id)
		if err != nil {
			return err
		}

		buf, err := json.MarshalIndent(&lock, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(buf))

		return nil
	}

	// load index, handle all the other types
	err = repo.LoadIndex()
	if err != nil {
		return err
	}

	switch tpe {
	case "pack":
		h := backend.Handle{Type: backend.Data, Name: id.String()}
		buf, err := backend.LoadAll(repo.Backend(), h, nil)
		if err != nil {
			return err
		}

		_, err = os.Stdout.Write(buf)
		return err

	case "blob":
		list, err := repo.Index().Lookup(id, pack.Data)
		if err != nil {
			return err
		}
		blob := list[0]

		buf := make([]byte, blob.Length)
		data, err := repo.LoadBlob(id, pack.Data, buf)
		if err != nil {
			return err
		}

		_, err = os.Stdout.Write(data)
		return err

	case "tree":
		debug.Log("cat", "cat tree %v", id.Str())
		tree := restic.NewTree()
		err = repo.LoadJSONPack(pack.Tree, id, tree)
		if err != nil {
			debug.Log("cat", "unable to load tree %v: %v", id.Str(), err)
			return err
		}

		buf, err := json.MarshalIndent(&tree, "", "  ")
		if err != nil {
			debug.Log("cat", "error json.MarshalIndent(): %v", err)
			return err
		}

		_, err = os.Stdout.Write(append(buf, '\n'))
		return nil

	default:
		return errors.New("invalid type")
	}
}
