package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/pack"
	"github.com/restic/restic/repository"
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
		rd, err := repo.Backend().Get(backend.Key, id.String())
		if err != nil {
			return err
		}

		dec := json.NewDecoder(rd)

		var key repository.Key
		err = dec.Decode(&key)
		if err != nil {
			return err
		}

		buf, err := json.MarshalIndent(&key, "", "  ")
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
		rd, err := repo.Backend().Get(backend.Data, id.String())
		if err != nil {
			return err
		}

		_, err = io.Copy(os.Stdout, rd)
		return err

	case "blob":
		blob, err := repo.Index().Lookup(id)
		if err != nil {
			return err
		}

		if blob.Type != pack.Data {
			return errors.New("wrong type for blob")
		}

		buf := make([]byte, blob.Length)
		data, err := repo.LoadBlob(pack.Data, id, buf)
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
