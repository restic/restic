package main

import (
	"context"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/feature"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/rechunker"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Reference: cmd_copy.go (v0.18.0)

func newRechunkCopyCommand(globalOptions *global.Options) *cobra.Command {
	var opts RechunkCopyOptions
	cmd := &cobra.Command{
		Use:   "rechunk-copy [flags] [snapshotID ...]",
		Short: "Rechunk-copy snapshots from one repository to another",
		Long: `
The "rechunk-copy" command rechunk-copies one or more snapshots from one repository to another.

Data blobs will be rechunked and stored in the destination repo. 
Tree blobs in the destination repo are also updated to point to the rechunked data blobs, 
but it does not modify any other metadata.

NOTE: This command has largely different internal mechanism from "copy" command,
due to restic's content defined chunking (CDC) algorithm. Note that "rechunk-copy"
could consume significantly more bandwidth during the process compared to "copy", 
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
			finalizeSnapshotFilter(&opts.SnapshotFilter)
			return runRechunkCopy(cmd.Context(), opts, *globalOptions, args, globalOptions.Term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// RechunkCopyOptions bundles all options for the rechunk-copy command.
type RechunkCopyOptions struct {
	global.SecondaryRepoOptions
	data.SnapshotFilter
	RechunkTags       data.TagLists
	CacheSize         int
	isIntegrationTest bool // skip check for RESTIC_FEATURES=rechunk-copy when integration test
}

func (opts *RechunkCopyOptions) AddFlags(f *pflag.FlagSet) {
	opts.SecondaryRepoOptions.AddFlags(f, "destination", "to copy snapshots from")
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)
	f.Var(&opts.RechunkTags, "rechunk-tag", "add `tags` for the copied snapshots in the format `tag[,tag,...]` (can be specified multiple times)")
	f.IntVar(&opts.CacheSize, "cache-size", 4096, "in-memory blob cache size in MiBs (0 to disable)")
}

func runRechunkCopy(ctx context.Context, opts RechunkCopyOptions, gopts global.Options, args []string, term ui.Terminal) error {
	if !feature.Flag.Enabled(feature.RechunkCopy) && !opts.isIntegrationTest {
		return errors.Fatal("rechunk-copy feature flag is not set. Currently, rechunk-copy is alpha feature (disabled by default).")
	}
	if opts.CacheSize != 0 && opts.CacheSize < 100 {
		return errors.Fatal("blob cache size must be at least 100 MiB")
	}

	printer := ui.NewProgressPrinter(false, gopts.Verbosity, term)
	secondaryGopts, isFromRepo, err := opts.SecondaryRepoOptions.FillGlobalOpts(ctx, gopts, "destination")
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

	if srcRepo.Config().ChunkerPolynomial == dstRepo.Config().ChunkerPolynomial {
		return errors.Fatal("source repo and destination repo have same chunker polynomials; use `restic copy` instead")
	}

	srcSnapshotLister, err := restic.MemorizeList(ctx, srcRepo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	debug.Log("Loading source index")
	if err := srcRepo.LoadIndex(ctx, printer); err != nil {
		return err
	}
	debug.Log("Loading destination index")
	if err := dstRepo.LoadIndex(ctx, printer); err != nil {
		return err
	}

	debug.Log("Running NewRechunker()")
	rechnker := rechunker.NewRechunker(dstRepo.Config().ChunkerPolynomial)
	rootTrees := []restic.ID{}

	// gather all root trees of snapshots for rechunking
	debug.Log("Gathering root trees of target snapshots")
	for sn := range FindFilteredSnapshots(ctx, srcSnapshotLister, srcRepo, &opts.SnapshotFilter, args, printer) {
		rootTrees = append(rootTrees, *sn.Tree)
	}

	// run rechunk process
	debug.Log("Running runRechunk()")
	progress := rechunker.NewProgress(
		term,
		printer,
		ui.CalculateProgressInterval(!gopts.Quiet, gopts.JSON, term.CanUpdateStatus()),
	)
	if err = runRechunk(ctx, srcRepo, rootTrees, dstRepo, rechnker, opts.CacheSize*(1<<20), printer, progress); err != nil {
		return err
	}

	// rewrite trees
	printer.P("Rewriting trees...\n")
	err = dstRepo.WithBlobUploader(ctx, func(ctx context.Context, uploader restic.BlobSaver) error {
		for sn := range FindFilteredSnapshots(ctx, srcSnapshotLister, srcRepo, &opts.SnapshotFilter, args, printer) {
			debug.Log("Running RewriteTree() for tree ID %v", sn.Tree.Str())
			_, err := rechnker.RewriteTree(ctx, srcRepo, uploader, *sn.Tree)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}
	printer.V("Rewriting done.\n\n")

	// write snapshots
	debug.Log("Writing snapshots")
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
		debug.Log("Snapshot %v: Original root tree %v is substituted with new %v", sn.ID().Str(), sn.Tree.Str(), newTreeID.Str())
		// change Tree field to new one
		sn.Tree = &newTreeID
		// add tags if provided by user
		sn.AddTags(opts.RechunkTags.Flatten())
		newID, err := data.SaveSnapshot(ctx, dstRepo, sn)
		if err != nil {
			return err
		}
		debug.Log("Snapshot %v (src repo) is rechunk-copied to snapshot %v (dst repo)", sn.ID().Str(), newID.Str())
		printer.P("snapshot %s saved\n", newID.Str())
	}

	// summary
	printer.V("\n[Post-run Summary]")
	printer.V("Number of distinct files processed: %v", rechnker.NumFiles())
	printer.V("  - Total size processed (including duplicate blobs): %v", ui.FormatBytes(rechnker.TotalSize()))
	printer.P("Additional data stored to the repository: %v", ui.FormatBytes(rechnker.TotalAddedToDstRepo()))

	return ctx.Err()
}

func runRechunk(ctx context.Context, srcRepo restic.Repository, roots []restic.ID, dstRepo restic.Repository, rechnker *rechunker.Rechunker, cacheSize int, printer progress.Printer, progress *rechunker.Progress) error {
	printer.V("Planning rechunk...\n")
	debug.Log("Running Plan()")
	err := rechnker.Plan(ctx, srcRepo, roots, cacheSize != 0)
	if err != nil {
		return err
	}
	printer.V("Planning done.")

	printer.V("\n[Pre-run Summary]")
	// num_snapshots, num_distinct_files, total_size, num_packs,
	printer.V("Number of snapshots: %v", len(roots))
	printer.V("Number of distinct files to process: %v", rechnker.NumFiles())
	printer.V("  - Total size (including duplicate blobs): %v", ui.FormatBytes(rechnker.TotalSize()))
	printer.V("Number of packs to download: %v\n\n", rechnker.PackCount())

	debug.Log("Running RechunkData()")
	progress.Start(rechnker.NumFiles(), rechnker.TotalSize())
	err = rechnker.RechunkData(ctx, srcRepo, dstRepo, cacheSize, progress)
	if err != nil {
		return err
	}
	progress.Done()

	printer.V("Rechunking done.\n\n")

	return nil
}
