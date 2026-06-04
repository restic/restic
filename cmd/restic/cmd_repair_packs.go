package main

import (
	"bytes"
	"context"
	"io"
	"os"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/spf13/cobra"
)

func newRepairPacksCommand(globalOptions *global.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "packs [packIDs...]",
		Short: "Salvage damaged pack files",
		Long: `
The "repair packs" command extracts intact blobs from the specified pack files, rebuilds
the index to remove the damaged pack files and removes the pack files from the repository.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRepairPacks(cmd.Context(), *globalOptions, globalOptions.Term, args)
		},
	}
	return cmd
}

func runRepairPacks(ctx context.Context, gopts global.Options, term ui.Terminal, args []string) error {
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

	printer := progress.NewTerminalPrinter(false, gopts.Verbosity, term)

	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, false, printer)
	if err != nil {
		return err
	}
	defer unlock()

	err = repo.LoadIndex(ctx, printer)
	if err != nil {
		return errors.Fatalf("%s", err)
	}

	blobsForPacks := restic.NewIDSet()
	for b := range repo.ListPacksFromIndex(ctx, ids) {
		if len(b.Blobs) == 0 {
			continue
		}
		// validate packfiles which have entries in the master index
		blobsForPacks.Insert(b.PackID)
	}

	printer.P("saving backup copies of pack files to current folder")
	for id := range ids {
		buf, err := repo.LoadRaw(ctx, restic.PackFile, id)
		// corrupted data is fine,but  must have valid index entries
		if buf == nil && !blobsForPacks.Has(id) {
			return err
		}

		f, err := os.OpenFile("pack-"+id.String(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, bytes.NewReader(buf)); err != nil {
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

	printer.E("\nUse `restic repair snapshots --forget` to remove the corrupted data blobs from all snapshots")
	return nil
}
