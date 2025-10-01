package main

import (
	"context"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/walker"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newRepairSnapshotsCommand() *cobra.Command {
	var opts RepairOptions

	cmd := &cobra.Command{
		Use:   "snapshots [flags] [snapshot ID] [...]",
		Short: "Repair snapshots",
		Long: `
The "repair snapshots" command repairs broken snapshots. It scans the given
snapshots and generates new ones with damaged directories and file contents
removed. If the broken snapshots are deleted, a prune run will be able to
clean up the repository.

The command depends on a correct index, thus make sure to run "repair index"
first!


WARNING
=======

Repairing and deleting broken snapshots causes data loss! It will remove broken
directories and modify broken files in the modified snapshots.

If the contents of directories and files are still available, the better option
is to run "backup" which in that case is able to heal existing snapshots. Only
use the "repair snapshots" command if you need to recover an old and broken
snapshot!

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
			finalizeSnapshotFilter(cmd.Flags(), &opts.SnapshotFilter)
			term, cancel := setupTermstatus()
			defer cancel()
			return runRepairSnapshots(cmd.Context(), globalOptions, opts, args, term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// RepairOptions collects all options for the repair command.
type RepairOptions struct {
	DryRun bool
	Forget bool

	restic.SnapshotFilter
}

func (opts *RepairOptions) AddFlags(f *pflag.FlagSet) {
	f.BoolVarP(&opts.DryRun, "dry-run", "n", false, "do not do anything, just print what would be done")
	f.BoolVarP(&opts.Forget, "forget", "", false, "remove original snapshots after creating new ones")

	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)
}

func runRepairSnapshots(ctx context.Context, gopts GlobalOptions, opts RepairOptions, args []string, term ui.Terminal) error {
	printer := newTerminalProgressPrinter(false, gopts.verbosity, term)

	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, opts.DryRun, printer)
	if err != nil {
		return err
	}
	defer unlock()

	snapshotLister, err := restic.MemorizeList(ctx, repo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	bar := newIndexTerminalProgress(printer)
	if err := repo.LoadIndex(ctx, bar); err != nil {
		return err
	}

	// Three error cases are checked:
	// - tree is a nil tree (-> will be replaced by an empty tree)
	// - trees which cannot be loaded (-> the tree contents will be removed)
	// - files whose contents are not fully available  (-> file will be modified)
	rewriter := walker.NewTreeRewriter(walker.RewriteOpts{
		RewriteNode: func(node *restic.Node, path string) *restic.Node {
			if node.Type == restic.NodeTypeIrregular || node.Type == restic.NodeTypeInvalid {
				printer.P("  file %q: removed node with invalid type %q", path, node.Type)
				return nil
			}
			if node.Type != restic.NodeTypeFile {
				return node
			}

			ok := true
			var newContent = restic.IDs{}
			var newSize uint64
			// check all contents and remove if not available
			for _, id := range node.Content {
				if size, found := repo.LookupBlobSize(restic.DataBlob, id); !found {
					ok = false
				} else {
					newContent = append(newContent, id)
					newSize += uint64(size)
				}
			}
			if !ok {
				printer.P("  file %q: removed missing content", path)
			} else if newSize != node.Size {
				printer.P("  file %q: fixed incorrect size", path)
			}
			// no-ops if already correct
			node.Content = newContent
			node.Size = newSize
			return node
		},
		RewriteFailedTree: func(_ restic.ID, path string, _ error) (restic.ID, error) {
			if path == "/" {
				printer.P("  dir %q: not readable", path)
				// remove snapshots with invalid root node
				return restic.ID{}, nil
			}
			// If a subtree fails to load, remove it
			printer.P("  dir %q: replaced with empty directory", path)
			emptyID, err := restic.SaveTree(ctx, repo, &restic.Tree{})
			if err != nil {
				return restic.ID{}, err
			}
			return emptyID, nil
		},
		AllowUnstableSerialization: true,
	})

	changedCount := 0
	for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, &opts.SnapshotFilter, args, printer) {
		printer.P("\n%v", sn)
		changed, err := filterAndReplaceSnapshot(ctx, repo, sn,
			func(ctx context.Context, sn *restic.Snapshot) (restic.ID, *restic.SnapshotSummary, error) {
				id, err := rewriter.RewriteTree(ctx, repo, "/", *sn.Tree)
				return id, nil, err
			}, opts.DryRun, opts.Forget, nil, "repaired", printer)
		if err != nil {
			return errors.Fatalf("unable to rewrite snapshot ID %q: %v", sn.ID().Str(), err)
		}
		if changed {
			changedCount++
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	printer.P("")
	if changedCount == 0 {
		if !opts.DryRun {
			printer.P("no snapshots were modified")
		} else {
			printer.P("no snapshots would be modified")
		}
	} else {
		if !opts.DryRun {
			printer.P("modified %v snapshots", changedCount)
		} else {
			printer.P("would modify %v snapshots", changedCount)
		}
	}

	return nil
}
