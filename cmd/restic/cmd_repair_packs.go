package main

import (
	"context"
	"io"
	"os"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/termstatus"
	"github.com/spf13/cobra"
)

var cmdRepairPacks = &cobra.Command{
	Use:   "packs [packIDs...]",
	Short: "Salvage damaged pack files",
	Long: `
WARNING: The CLI for this command is experimental and will likely change in the future!

The "repair packs" command extracts intact blobs from the specified pack files, rebuilds
the index to remove the damaged pack files and removes the pack files from the repository.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		term, cancel := setupTermstatus()
		defer cancel()
		return runRepairPacks(cmd.Context(), globalOptions, term, args)
	},
}

func init() {
	cmdRepair.AddCommand(cmdRepairPacks)
}

func runRepairPacks(ctx context.Context, gopts GlobalOptions, term *termstatus.Terminal, args []string) error {
	ids := restic.NewIDSet()
	for _, arg := range args {
		id, err := restic.ParseID(arg)
		if err != nil {
			return err
		}
		ids.Insert(id)
	}
	if len(ids) == 0 {
		return errors.Fatal("no ids specified")
	}

	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, false)
	if err != nil {
		return err
	}
	defer unlock()

	printer := newTerminalProgressPrinter(gopts.verbosity, term)

	bar := newIndexTerminalProgress(gopts.Quiet, gopts.JSON, term)
	err = repo.LoadIndex(ctx, bar)
	if err != nil {
		return errors.Fatalf("%s", err)
	}

	printer.P("saving backup copies of pack files to current folder")
	for id := range ids {
		f, err := os.OpenFile("pack-"+id.String(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if err != nil {
			return err
		}

		err = repo.Backend().Load(ctx, backend.Handle{Type: restic.PackFile, Name: id.String()}, 0, 0, func(rd io.Reader) error {
			_, err := f.Seek(0, 0)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, rd)
			return err
		})
		if err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}

	err = repository.RepairPacks(ctx, repo, ids, printer)
	if err != nil {
		return errors.Fatalf("%s", err)
	}

	Warnf("\nUse `restic repair snapshots --forget` to remove the corrupted data blobs from all snapshots\n")
	return nil
}
