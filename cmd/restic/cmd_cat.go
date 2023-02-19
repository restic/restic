package main

import (
	"context"
	"encoding/json"

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

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCat(cmd.Context(), globalOptions, args)
	},
}

func init() {
	cmdRoot.AddCommand(cmdCat)
}

func runCat(ctx context.Context, gopts GlobalOptions, args []string) error {
	if len(args) < 1 || (args[0] != "masterkey" && args[0] != "config" && len(args) != 2) {
		return errors.Fatal("type or ID not specified")
	}

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		var lock *restic.Lock
		lock, ctx, err = lockRepo(ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	tpe := args[0]

	var id restic.ID
	if tpe != "masterkey" && tpe != "config" && tpe != "snapshot" {
		id, err = restic.ParseID(args[1])
		if err != nil {
			return errors.Fatalf("unable to parse ID: %v\n", err)
		}
	}

	switch tpe {
	case "config":
		buf, err := json.MarshalIndent(repo.Config(), "", "  ")
		if err != nil {
			return err
		}

		Println(string(buf))
		return nil
	case "index":
		buf, err := repo.LoadUnpacked(ctx, restic.IndexFile, id)
		if err != nil {
			return err
		}

		Println(string(buf))
		return nil
	case "snapshot":
		sn, err := restic.FindSnapshot(ctx, repo.Backend(), repo, args[1])
		if err != nil {
			return errors.Fatalf("could not find snapshot: %v\n", err)
		}

		buf, err := json.MarshalIndent(sn, "", "  ")
		if err != nil {
			return err
		}

		Println(string(buf))
		return nil
	case "key":
		key, err := repository.LoadKey(ctx, repo, id)
		if err != nil {
			return err
		}

		buf, err := json.MarshalIndent(&key, "", "  ")
		if err != nil {
			return err
		}

		Println(string(buf))
		return nil
	case "masterkey":
		buf, err := json.MarshalIndent(repo.Key(), "", "  ")
		if err != nil {
			return err
		}

		Println(string(buf))
		return nil
	case "lock":
		lock, err := restic.LoadLock(ctx, repo, id)
		if err != nil {
			return err
		}

		buf, err := json.MarshalIndent(&lock, "", "  ")
		if err != nil {
			return err
		}

		Println(string(buf))
		return nil

	case "pack":
		h := restic.Handle{Type: restic.PackFile, Name: id.String()}
		buf, err := backend.LoadAll(ctx, nil, repo.Backend(), h)
		if err != nil {
			return err
		}

		hash := restic.Hash(buf)
		if !hash.Equal(id) {
			Warnf("Warning: hash of data does not match ID, want\n  %v\ngot:\n  %v\n", id.String(), hash.String())
		}

		_, err = globalOptions.stdout.Write(buf)
		return err

	case "blob":
		err = repo.LoadIndex(ctx)
		if err != nil {
			return err
		}

		for _, t := range []restic.BlobType{restic.DataBlob, restic.TreeBlob} {
			bh := restic.BlobHandle{ID: id, Type: t}
			if !repo.Index().Has(bh) {
				continue
			}

			buf, err := repo.LoadBlob(ctx, t, id, nil)
			if err != nil {
				return err
			}

			_, err = globalOptions.stdout.Write(buf)
			return err
		}

		return errors.Fatal("blob not found")

	default:
		return errors.Fatal("invalid type")
	}
}
