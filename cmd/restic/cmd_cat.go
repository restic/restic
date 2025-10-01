package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
)

var catAllowedCmds = []string{"config", "index", "snapshot", "key", "masterkey", "lock", "pack", "blob", "tree"}

func newCatCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cat [flags] [masterkey|config|pack ID|blob ID|snapshot ID|index ID|key ID|lock ID|tree snapshot:subfolder]",
		Short: "Print internal objects to stdout",
		Long: `
The "cat" command is used to print internal objects to stdout.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
		GroupID:           cmdGroupDefault,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			term, cancel := setupTermstatus()
			defer cancel()
			return runCat(cmd.Context(), globalOptions, args, term)
		},
		ValidArgs: catAllowedCmds,
	}
	return cmd
}

func validateCatArgs(args []string) error {
	if len(args) < 1 {
		return errors.Fatal("type not specified")
	}

	validType := false
	for _, v := range catAllowedCmds {
		if v == args[0] {
			validType = true
			break
		}
	}
	if !validType {
		return errors.Fatalf("invalid type %q, must be one of [%s]", args[0], strings.Join(catAllowedCmds, "|"))
	}

	if args[0] != "masterkey" && args[0] != "config" && len(args) != 2 {
		return errors.Fatal("ID not specified")
	}

	return nil
}

func runCat(ctx context.Context, gopts GlobalOptions, args []string, term ui.Terminal) error {
	printer := newTerminalProgressPrinter(gopts.JSON, gopts.verbosity, term)

	if err := validateCatArgs(args); err != nil {
		return err
	}

	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	tpe := args[0]

	var id restic.ID
	if tpe != "masterkey" && tpe != "config" && tpe != "snapshot" && tpe != "tree" {
		id, err = restic.ParseID(args[1])
		if err != nil {
			return errors.Fatalf("unable to parse ID: %v", err)
		}
	}

	switch tpe {
	case "config":
		buf, err := json.MarshalIndent(repo.Config(), "", "  ")
		if err != nil {
			return err
		}

		printer.S(string(buf))
		return nil
	case "index":
		buf, err := repo.LoadUnpacked(ctx, restic.IndexFile, id)
		if err != nil {
			return err
		}

		printer.S(string(buf))
		return nil
	case "snapshot":
		sn, _, err := restic.FindSnapshot(ctx, repo, repo, args[1])
		if err != nil {
			return errors.Fatalf("could not find snapshot: %v", err)
		}

		buf, err := json.MarshalIndent(sn, "", "  ")
		if err != nil {
			return err
		}

		printer.S(string(buf))
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

		printer.S(string(buf))
		return nil
	case "masterkey":
		buf, err := json.MarshalIndent(repo.Key(), "", "  ")
		if err != nil {
			return err
		}

		printer.S(string(buf))
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

		printer.S(string(buf))
		return nil

	case "pack":
		buf, err := repo.LoadRaw(ctx, restic.PackFile, id)
		// allow returning broken pack files
		if buf == nil {
			return err
		}

		hash := restic.Hash(buf)
		if !hash.Equal(id) {
			printer.E("Warning: hash of data does not match ID, want\n  %v\ngot:\n  %v", id.String(), hash.String())
		}

		_, err = term.OutputRaw().Write(buf)
		return err

	case "blob":
		bar := newIndexTerminalProgress(printer)
		err = repo.LoadIndex(ctx, bar)
		if err != nil {
			return err
		}

		for _, t := range []restic.BlobType{restic.DataBlob, restic.TreeBlob} {
			if _, ok := repo.LookupBlobSize(t, id); !ok {
				continue
			}

			buf, err := repo.LoadBlob(ctx, t, id, nil)
			if err != nil {
				return err
			}

			_, err = term.OutputRaw().Write(buf)
			return err
		}

		return errors.Fatal("blob not found")

	case "tree":
		sn, subfolder, err := restic.FindSnapshot(ctx, repo, repo, args[1])
		if err != nil {
			return errors.Fatalf("could not find snapshot: %v", err)
		}

		bar := newIndexTerminalProgress(printer)
		err = repo.LoadIndex(ctx, bar)
		if err != nil {
			return err
		}

		sn.Tree, err = restic.FindTreeDirectory(ctx, repo, sn.Tree, subfolder)
		if err != nil {
			return err
		}

		buf, err := repo.LoadBlob(ctx, restic.TreeBlob, *sn.Tree, nil)
		if err != nil {
			return err
		}
		_, err = term.OutputRaw().Write(buf)
		return err

	default:
		return errors.Fatal("invalid type")
	}
}
