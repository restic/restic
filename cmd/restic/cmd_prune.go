package main

import (
	"context"
	"math"
	"runtime"
	"strconv"
	"strings"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/restic/restic/internal/ui/termstatus"

	"github.com/spf13/cobra"
)

var cmdPrune = &cobra.Command{
	Use:   "prune [flags]",
	Short: "Remove unneeded data from the repository",
	Long: `
The "prune" command checks the repository and removes data that is not
referenced and therefore not needed any more.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		term, cancel := setupTermstatus()
		defer cancel()
		return runPrune(cmd.Context(), pruneOptions, globalOptions, term)
	},
}

// PruneOptions collects all options for the cleanup command.
type PruneOptions struct {
	DryRun                bool
	UnsafeNoSpaceRecovery string

	unsafeRecovery bool

	MaxUnused      string
	maxUnusedBytes func(used uint64) (unused uint64) // calculates the number of unused bytes after repacking, according to MaxUnused

	MaxRepackSize  string
	MaxRepackBytes uint64

	RepackCacheableOnly bool
	RepackSmall         bool
	RepackUncompressed  bool
}

var pruneOptions PruneOptions

func init() {
	cmdRoot.AddCommand(cmdPrune)
	f := cmdPrune.Flags()
	f.BoolVarP(&pruneOptions.DryRun, "dry-run", "n", false, "do not modify the repository, just print what would be done")
	f.StringVarP(&pruneOptions.UnsafeNoSpaceRecovery, "unsafe-recover-no-free-space", "", "", "UNSAFE, READ THE DOCUMENTATION BEFORE USING! Try to recover a repository stuck with no free space. Do not use without trying out 'prune --max-repack-size 0' first.")
	addPruneOptions(cmdPrune, &pruneOptions)
}

func addPruneOptions(c *cobra.Command, pruneOptions *PruneOptions) {
	f := c.Flags()
	f.StringVar(&pruneOptions.MaxUnused, "max-unused", "5%", "tolerate given `limit` of unused data (absolute value in bytes with suffixes k/K, m/M, g/G, t/T, a value in % or the word 'unlimited')")
	f.StringVar(&pruneOptions.MaxRepackSize, "max-repack-size", "", "maximum `size` to repack (allowed suffixes: k/K, m/M, g/G, t/T)")
	f.BoolVar(&pruneOptions.RepackCacheableOnly, "repack-cacheable-only", false, "only repack packs which are cacheable")
	f.BoolVar(&pruneOptions.RepackSmall, "repack-small", false, "repack pack files below 80% of target pack size")
	f.BoolVar(&pruneOptions.RepackUncompressed, "repack-uncompressed", false, "repack all uncompressed data")
}

func verifyPruneOptions(opts *PruneOptions) error {
	opts.MaxRepackBytes = math.MaxUint64
	if len(opts.MaxRepackSize) > 0 {
		size, err := ui.ParseBytes(opts.MaxRepackSize)
		if err != nil {
			return err
		}
		opts.MaxRepackBytes = uint64(size)
	}
	if opts.UnsafeNoSpaceRecovery != "" {
		// prevent repacking data to make sure users cannot get stuck.
		opts.MaxRepackBytes = 0
	}

	maxUnused := strings.TrimSpace(opts.MaxUnused)
	if maxUnused == "" {
		return errors.Fatalf("invalid value for --max-unused: %q", opts.MaxUnused)
	}

	// parse MaxUnused either as unlimited, a percentage, or an absolute number of bytes
	switch {
	case maxUnused == "unlimited":
		opts.maxUnusedBytes = func(_ uint64) uint64 {
			return math.MaxUint64
		}

	case strings.HasSuffix(maxUnused, "%"):
		maxUnused = strings.TrimSuffix(maxUnused, "%")
		p, err := strconv.ParseFloat(maxUnused, 64)
		if err != nil {
			return errors.Fatalf("invalid percentage %q passed for --max-unused: %v", opts.MaxUnused, err)
		}

		if p < 0 {
			return errors.Fatal("percentage for --max-unused must be positive")
		}

		if p >= 100 {
			return errors.Fatal("percentage for --max-unused must be below 100%")
		}

		opts.maxUnusedBytes = func(used uint64) uint64 {
			return uint64(p / (100 - p) * float64(used))
		}

	default:
		size, err := ui.ParseBytes(maxUnused)
		if err != nil {
			return errors.Fatalf("invalid number of bytes %q for --max-unused: %v", opts.MaxUnused, err)
		}

		opts.maxUnusedBytes = func(_ uint64) uint64 {
			return uint64(size)
		}
	}

	return nil
}

func runPrune(ctx context.Context, opts PruneOptions, gopts GlobalOptions, term *termstatus.Terminal) error {
	err := verifyPruneOptions(&opts)
	if err != nil {
		return err
	}

	if opts.RepackUncompressed && gopts.Compression == repository.CompressionOff {
		return errors.Fatal("disabled compression and `--repack-uncompressed` are mutually exclusive")
	}

	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, false)
	if err != nil {
		return err
	}
	defer unlock()

	if opts.UnsafeNoSpaceRecovery != "" {
		repoID := repo.Config().ID
		if opts.UnsafeNoSpaceRecovery != repoID {
			return errors.Fatalf("must pass id '%s' to --unsafe-recover-no-free-space", repoID)
		}
		opts.unsafeRecovery = true
	}

	return runPruneWithRepo(ctx, opts, gopts, repo, restic.NewIDSet(), term)
}

func runPruneWithRepo(ctx context.Context, opts PruneOptions, gopts GlobalOptions, repo *repository.Repository, ignoreSnapshots restic.IDSet, term *termstatus.Terminal) error {
	if repo.Cache == nil {
		Print("warning: running prune without a cache, this may be very slow!\n")
	}

	printer := newTerminalProgressPrinter(gopts.verbosity, term)

	printer.P("loading indexes...\n")
	// loading the index before the snapshots is ok, as we use an exclusive lock here
	bar := newIndexTerminalProgress(gopts.Quiet, gopts.JSON, term)
	err := repo.LoadIndex(ctx, bar)
	if err != nil {
		return err
	}

	popts := repository.PruneOptions{
		DryRun:         opts.DryRun,
		UnsafeRecovery: opts.unsafeRecovery,

		MaxUnusedBytes: opts.maxUnusedBytes,
		MaxRepackBytes: opts.MaxRepackBytes,

		RepackCacheableOnly: opts.RepackCacheableOnly,
		RepackSmall:         opts.RepackSmall,
		RepackUncompressed:  opts.RepackUncompressed,
	}

	plan, err := repository.PlanPrune(ctx, popts, repo, func(ctx context.Context, repo restic.Repository, usedBlobs restic.FindBlobSet) error {
		return getUsedBlobs(ctx, repo, usedBlobs, ignoreSnapshots, printer)
	}, printer)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if popts.DryRun {
		printer.P("\nWould have made the following changes:")
	}

	err = printPruneStats(printer, plan.Stats())
	if err != nil {
		return err
	}

	// Trigger GC to reset garbage collection threshold
	runtime.GC()

	return plan.Execute(ctx, printer)
}

// printPruneStats prints out the statistics
func printPruneStats(printer progress.Printer, stats repository.PruneStats) error {
	printer.V("\nused:         %10d blobs / %s\n", stats.Blobs.Used, ui.FormatBytes(stats.Size.Used))
	if stats.Blobs.Duplicate > 0 {
		printer.V("duplicates:   %10d blobs / %s\n", stats.Blobs.Duplicate, ui.FormatBytes(stats.Size.Duplicate))
	}
	printer.V("unused:       %10d blobs / %s\n", stats.Blobs.Unused, ui.FormatBytes(stats.Size.Unused))
	if stats.Size.Unref > 0 {
		printer.V("unreferenced:                    %s\n", ui.FormatBytes(stats.Size.Unref))
	}
	totalBlobs := stats.Blobs.Used + stats.Blobs.Unused + stats.Blobs.Duplicate
	totalSize := stats.Size.Used + stats.Size.Duplicate + stats.Size.Unused + stats.Size.Unref
	unusedSize := stats.Size.Duplicate + stats.Size.Unused
	printer.V("total:        %10d blobs / %s\n", totalBlobs, ui.FormatBytes(totalSize))
	printer.V("unused size: %s of total size\n", ui.FormatPercent(unusedSize, totalSize))

	printer.P("\nto repack:    %10d blobs / %s\n", stats.Blobs.Repack, ui.FormatBytes(stats.Size.Repack))
	printer.P("this removes: %10d blobs / %s\n", stats.Blobs.Repackrm, ui.FormatBytes(stats.Size.Repackrm))
	printer.P("to delete:    %10d blobs / %s\n", stats.Blobs.Remove, ui.FormatBytes(stats.Size.Remove+stats.Size.Unref))
	totalPruneSize := stats.Size.Remove + stats.Size.Repackrm + stats.Size.Unref
	printer.P("total prune:  %10d blobs / %s\n", stats.Blobs.Remove+stats.Blobs.Repackrm, ui.FormatBytes(totalPruneSize))
	if stats.Size.Uncompressed > 0 {
		printer.P("not yet compressed:              %s\n", ui.FormatBytes(stats.Size.Uncompressed))
	}
	printer.P("remaining:    %10d blobs / %s\n", totalBlobs-(stats.Blobs.Remove+stats.Blobs.Repackrm), ui.FormatBytes(totalSize-totalPruneSize))
	unusedAfter := unusedSize - stats.Size.Remove - stats.Size.Repackrm
	printer.P("unused size after prune: %s (%s of remaining size)\n",
		ui.FormatBytes(unusedAfter), ui.FormatPercent(unusedAfter, totalSize-totalPruneSize))
	printer.P("\n")
	printer.V("totally used packs: %10d\n", stats.Packs.Used)
	printer.V("partly used packs:  %10d\n", stats.Packs.PartlyUsed)
	printer.V("unused packs:       %10d\n\n", stats.Packs.Unused)

	printer.V("to keep:      %10d packs\n", stats.Packs.Keep)
	printer.V("to repack:    %10d packs\n", stats.Packs.Repack)
	printer.V("to delete:    %10d packs\n", stats.Packs.Remove)
	if stats.Packs.Unref > 0 {
		printer.V("to delete:    %10d unreferenced packs\n\n", stats.Packs.Unref)
	}
	return nil
}

func getUsedBlobs(ctx context.Context, repo restic.Repository, usedBlobs restic.FindBlobSet, ignoreSnapshots restic.IDSet, printer progress.Printer) error {
	var snapshotTrees restic.IDs
	printer.P("loading all snapshots...\n")
	err := restic.ForAllSnapshots(ctx, repo, repo, ignoreSnapshots,
		func(id restic.ID, sn *restic.Snapshot, err error) error {
			if err != nil {
				debug.Log("failed to load snapshot %v (error %v)", id, err)
				return err
			}
			debug.Log("add snapshot %v (tree %v)", id, *sn.Tree)
			snapshotTrees = append(snapshotTrees, *sn.Tree)
			return nil
		})
	if err != nil {
		return errors.Fatalf("failed loading snapshot: %v", err)
	}

	printer.P("finding data that is still in use for %d snapshots\n", len(snapshotTrees))

	bar := printer.NewCounter("snapshots")
	bar.SetMax(uint64(len(snapshotTrees)))
	defer bar.Done()

	return restic.FindUsedBlobs(ctx, repo, snapshotTrees, usedBlobs, bar)
}
