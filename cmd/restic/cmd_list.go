package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"

	"github.com/spf13/cobra"
)

func newListCommand(globalOptions *global.Options) *cobra.Command {
	var listAllowedArgs = []string{"blobs", "packs", "index", "snapshots", "keys", "locks"}
	var listAllowedArgsUseString = strings.Join(listAllowedArgs, "|")

	cmd := &cobra.Command{
		Use:   "list [flags] [" + listAllowedArgsUseString + "] or list packs snapshotID",
		Short: "List objects in the repository",
		Long: `
The "list" command allows listing objects in the repository based on type.
The "list packs snapshotID" accepts one snapshotID and lists all packfiles
used by this snapshot.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
		DisableAutoGenTag: true,
		GroupID:           cmdGroupDefault,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), *globalOptions, args, globalOptions.Term, listAllowedArgsUseString)
		},
		ValidArgs: listAllowedArgs,
	}
	return cmd
}

func runList(ctx context.Context, gopts global.Options, args []string, term ui.Terminal,
	listAllowedArgsUseString string,
) error {
	printer := progress.NewTerminalPrinter(false, gopts.Verbosity, term)

	if len(args) == 0 {
		printer.E("restic list needs one of [%s] or `list packs snapshotID` as argument", listAllowedArgsUseString)
		return errors.Fatal("type not specified")
	}

	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock || args[0] == "locks", printer)
	if err != nil {
		return err
	}
	defer unlock()

	var t restic.FileType
	switch args[0] {
	case "packs":
		t = restic.PackFile
		if len(args) == 2 {
			// args[1] needs to be a snapshotID
			return packfileLIst(ctx, repo, args[1], printer)
		}
	case "index":
		t = restic.IndexFile
	case "snapshots":
		t = restic.SnapshotFile
	case "keys":
		t = restic.KeyFile
	case "locks":
		t = restic.LockFile
	case "blobs":
		for entry := range repository.AllIndexBlobs(ctx, repo, repo) {
			if entry.Error != nil {
				return entry.Error
			}
			printer.S("%v %v", entry.Handle.Type, entry.Handle.ID)
		}
		return nil
	default:
		return errors.Fatal("invalid type")
	}

	return repo.List(ctx, t, func(id restic.ID, _ int64) error {
		printer.S("%s", id)
		return nil
	})
}

// packfileLIst runs the list packs <snapshotID>
// it creates a sorted list of packfiles belonging to this snapshot
func packfileLIst(ctx context.Context, repo restic.Repository, snapshotID string, printer progress.Printer) error {

	snapshotLister, err := restic.MemorizeList(ctx, repo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	sn, _, err := (&data.SnapshotFilter{}).FindLatest(ctx, snapshotLister, repo, snapshotID)
	if err != nil {
		return fmt.Errorf("required snapshot ID %q not found", snapshotID)
	}

	if err = repo.LoadIndex(ctx, printer); err != nil {
		return err
	}

	usedBlobs := repo.NewAssociatedBlobSet()
	bar := printer.NewCounter("snapshot")
	bar.SetMax(uint64(1))
	if err := data.FindUsedBlobs(ctx, repo, []restic.ID{*sn.Tree}, usedBlobs, bar); err != nil {
		return err
	}
	bar.Done()

	snapPacks := restic.NewIDSet()
	for bh := range usedBlobs.Keys() {
		for _, blob := range repo.LookupBlob(bh) {
			snapPacks.Insert(blob.PackID())
		}
	}

	for _, packID := range snapPacks.List() {
		printer.S("%v", packID)
	}

	return nil
}
