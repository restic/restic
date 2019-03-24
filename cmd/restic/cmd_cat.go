package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

var cmdCat = &cobra.Command{
	Use:   "cat [flags] [pack|blob|snapshot|index|key|masterkey|config|lock] ID",
	Short: "Print internal objects to stdout",
	Long: `
The "cat" command is used to print internal objects to stdout.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCat(globalOptions, args)
	},
}

func init() {
	cmdRoot.AddCommand(cmdCat)
}

func runCat(gopts GlobalOptions, args []string) error {
	if len(args) < 1 || (args[0] != "masterkey" && args[0] != "config" && len(args) != 2) {
		return errors.Fatal("type or ID not specified")
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	lock, err := lockRepo(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	tpe := args[0]

	var id restic.ID
	if tpe != "masterkey" && tpe != "config" {
		id, err = restic.ParseID(args[1])
		if err != nil {
			if tpe != "snapshot" {
				return errors.Fatalf("unable to parse ID: %v\n", err)
			}

			// find snapshot id with prefix
			id, err = restic.FindSnapshot(repo, args[1])
			if err != nil {
				return errors.Fatalf("could not find snapshot: %v\n", err)
			}
		}
	}

	// handle all types that don't need an index
	switch tpe {
	case "config":
		buf, err := json.MarshalIndent(repo.Config(), "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(buf))
		return nil
	case "index":
		buf, err := repo.LoadAndDecrypt(gopts.ctx, nil, restic.IndexFile, id)
		if err != nil {
			return err
		}

		_, err = os.Stdout.Write(append(buf, '\n'))
		return err

	case "snapshot":
		sn := &restic.Snapshot{}
		err = repo.LoadJSONUnpacked(gopts.ctx, restic.SnapshotFile, id, sn)
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
		h := restic.Handle{Type: restic.KeyFile, Name: id.String()}
		buf, err := backend.LoadAll(gopts.ctx, nil, repo.Backend(), h)
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
		lock, err := restic.LoadLock(gopts.ctx, repo, id)
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
	err = repo.LoadIndex(gopts.ctx)
	if err != nil {
		return err
	}

	switch tpe {
	case "pack":
		h := restic.Handle{Type: restic.DataFile, Name: id.String()}
		buf, err := backend.LoadAll(gopts.ctx, nil, repo.Backend(), h)
		if err != nil {
			return err
		}

		hash := restic.Hash(buf)
		if !hash.Equal(id) {
			fmt.Fprintf(stderr, "Warning: hash of data does not match ID, want\n  %v\ngot:\n  %v\n", id.String(), hash.String())
		}

		_, err = os.Stdout.Write(buf)
		return err

	case "blob":
		for _, t := range []restic.BlobType{restic.DataBlob, restic.TreeBlob} {
			list, found := repo.Index().Lookup(id, t)
			if !found {
				continue
			}
			blob := list[0]

			buf := make([]byte, blob.Length)
			n, err := repo.LoadBlob(gopts.ctx, t, id, buf)
			if err != nil {
				return err
			}
			buf = buf[:n]

			_, err = os.Stdout.Write(buf)
			return err
		}

		return errors.Fatal("blob not found")

	default:
		return errors.Fatal("invalid type")
	}
}
