package main

import (
	"context"
	"strings"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository/index"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"

	"github.com/spf13/cobra"
)

func newListCommand(globalOptions *global.Options) *cobra.Command {
	var listAllowedArgs = []string{"blobs", "packs", "index", "snapshots", "keys", "locks"}
	var listAllowedArgsUseString = strings.Join(listAllowedArgs, "|")

	cmd := &cobra.Command{
		Use:   "list [flags] [" + listAllowedArgsUseString + "]",
		Short: "List objects in the repository",
		Long: `
The "list" command allows listing objects in the repository based on type.

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
			return runList(cmd.Context(), *globalOptions, args, globalOptions.Term)
		},
		ValidArgs: listAllowedArgs,
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	}
	return cmd
}

func runList(ctx context.Context, gopts global.Options, args []string, term ui.Terminal) error {
	printer := ui.NewProgressPrinter(false, gopts.Verbosity, term)

	if len(args) != 1 {
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
	case "index":
		t = restic.IndexFile
	case "snapshots":
		t = restic.SnapshotFile
	case "keys":
		t = restic.KeyFile
	case "locks":
		t = restic.LockFile
	case "blobs":
		return index.ForAllIndexes(ctx, repo, repo, func(_ restic.ID, idx *index.Index, err error) error {
			if err != nil {
				return err
			}
			return idx.Each(ctx, func(blobs restic.PackedBlob) {
				printer.S("%v %v", blobs.Type, blobs.ID)
			})
		})
	default:
		return errors.Fatal("invalid type")
	}

	return repo.List(ctx, t, func(id restic.ID, _ int64) error {
		printer.S("%s", id)
		return nil
	})
}
