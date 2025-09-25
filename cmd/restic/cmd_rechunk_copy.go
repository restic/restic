package main

import (
	"context"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/feature"
	"github.com/restic/restic/internal/rechunker"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	"golang.org/x/sync/errgroup"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Reference: cmd_copy.go (v0.18.0)

func newRechunkCopyCommand() *cobra.Command {
	var opts RechunkCopyOptions
	cmd := &cobra.Command{
		Use:   "rechunk-copy [flags] [snapshotID ...]",
		Short: "Rechunk-copy snapshots from one repository to another",
		Long: `
The "rechunk-copy" command rechunk-copies one or more snapshots from one repository to another.

Data blobs stored in the destination repo are rechunked, and tree blobs in the destination repo are also updated accordingly.

NOTE: This command has largely different internal mechanism from "copy" command,
due to restic's content defined chunking (CDC) design. Note that "rechunk-copy"
may consume significantly more bandwidth during the process compared to "copy", 
and may also need significantly more time to finish.

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
			return runRechunkCopy(cmd.Context(), opts, globalOptions, args, term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// RechunkCopyOptions bundles all options for the rechunk-copy command.
type RechunkCopyOptions struct {
	secondaryRepoOptions
	restic.SnapshotFilter
	RechunkTags       restic.TagLists
	UsePackCache      bool
	isIntegrationTest bool // skip check for RESTIC_FEATURES=rechunk-copy when integration test
}

func (opts *RechunkCopyOptions) AddFlags(f *pflag.FlagSet) {
	opts.secondaryRepoOptions.AddFlags(f, "destination", "to copy snapshots from")
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)
	f.Var(&opts.RechunkTags, "rechunk-tag", "add `tags` for the copied snapshots in the format `tag[,tag,...]` (can be specified multiple times)")
	f.BoolVar(&opts.UsePackCache, "use-pack-cache", false, "use pack cache for remote source repository")
}

func runRechunkCopy(ctx context.Context, opts RechunkCopyOptions, gopts GlobalOptions, args []string, term ui.Terminal) error {
	if !feature.Flag.Enabled(feature.RechunkCopy) && !opts.isIntegrationTest {
		return errors.Fatal("rechunk-copy feature flag is not set. Currently, rechunk-copy is alpha feature (disabled by default).")
	}

	printer := newTerminalProgressPrinter(false, gopts.verbosity, term)
	secondaryGopts, isFromRepo, err := fillSecondaryGlobalOpts(ctx, opts.secondaryRepoOptions, gopts, "destination", printer)
	if err != nil {
		return err
	}
	if isFromRepo {
		// swap global options, if the secondary repo was set via from-repo
		gopts, secondaryGopts = secondaryGopts, gopts
	}

	ctx, srcRepo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	ctx, dstRepo, unlock, err := openWithAppendLock(ctx, secondaryGopts, false, printer)
	if err != nil {
		return err
	}
	defer unlock()

	srcSnapshotLister, err := restic.MemorizeList(ctx, srcRepo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	debug.Log("Loading source index")
	bar := newIndexTerminalProgress(printer)
	if err := srcRepo.LoadIndex(ctx, bar); err != nil {
		return err
	}
	bar = newIndexTerminalProgress(printer)
	debug.Log("Loading destination index")
	if err := dstRepo.LoadIndex(ctx, bar); err != nil {
		return err
	}

	rechnker := rechunker.NewRechunker(dstRepo.Config().ChunkerPolynomial)
	rootTrees := []restic.ID{}

	// first pass: gather all root trees of snapshots for rechunking
	for sn := range FindFilteredSnapshots(ctx, srcSnapshotLister, srcRepo, &opts.SnapshotFilter, args, printer) {
		rootTrees = append(rootTrees, *sn.Tree)
	}

	wg, wgCtx := errgroup.WithContext(ctx)
	dstRepo.StartPackUploader(wgCtx, wg)
	if err = runRechunk(ctx, srcRepo, rootTrees, dstRepo, rechnker, opts.UsePackCache, printer); err != nil {
		return err
	}

	// second pass: rewrite trees
	printer.V("Rewriting trees...\n")
	for sn := range FindFilteredSnapshots(ctx, srcSnapshotLister, srcRepo, &opts.SnapshotFilter, args, printer) {
		_, err := rechnker.RewriteTree(ctx, srcRepo, dstRepo, *sn.Tree)
		if err != nil {
			return err
		}
	}

	if err = dstRepo.Flush(wgCtx); err != nil {
		return err
	}

	printer.V("Rewriting done.\n\n")

	// third pass: write snapshots
	for sn := range FindFilteredSnapshots(ctx, srcSnapshotLister, srcRepo, &opts.SnapshotFilter, args, printer) {
		sn.Parent = nil // Parent does not have relevance in the new repo.
		// Use Original as a persistent snapshot ID
		if sn.Original == nil {
			sn.Original = sn.ID()
		}

		newTreeID, err := rechnker.GetRewrittenTree(*sn.Tree)
		if err != nil {
			return err
		}
		// change Tree field to new one
		sn.Tree = &newTreeID
		// add tags if provided by user
		sn.AddTags(opts.RechunkTags.Flatten())
		newID, err := restic.SaveSnapshot(ctx, dstRepo, sn)
		if err != nil {
			return err
		}
		printer.P("snapshot %s saved\n", newID.Str())
	}
	return ctx.Err()
}

func runRechunk(ctx context.Context, srcRepo restic.Repository, roots []restic.ID, dstRepo restic.Repository, rechnker *rechunker.Rechunker, usePackCache bool, printer progress.Printer) error {
	printer.V("Rechunk scheduling start...\n")
	err := rechnker.Plan(ctx, srcRepo, roots, usePackCache)
	if err != nil {
		return err
	}
	printer.V("Scheduling Done. Rechunking data...\n")

	bar := printer.NewCounter("distinct files rechunked")
	bar.SetMax(uint64(rechnker.NumFilesToProcess()))
	err = rechnker.RechunkData(ctx, srcRepo, dstRepo, bar)
	if err != nil {
		return err
	}
	bar.Done()

	printer.V("Rechunking done.\n\n")

	return nil
}
