package main

import (
	"context"
	"math"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newPruneCommand(globalOptions *global.Options) *cobra.Command {
	var opts PruneOptions

	cmd := &cobra.Command{
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
Exit status is 12 if the password is incorrect.
`,
		GroupID:           cmdGroupDefault,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPrune(cmd.Context(), opts, *globalOptions, globalOptions.Term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
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
	RepackUncompressed  bool

	SmallPackSize  string
	SmallPackBytes uint64

	// GroupBy clusters repacked tree (metadata) blobs of the same snapshot
	// group into shared pack files. When empty (the default) prune does not
	// group and behaves like a classic prune. It should match the
	// `backup --group-by` value used by the clients. It is not part of the
	// limited flag set because `forget` already owns an unrelated `--group-by`
	// flag (which groups snapshots for retention, not tree blobs for repacking).
	GroupBy data.SnapshotGroupByOptions
}

func (opts *PruneOptions) AddFlags(f *pflag.FlagSet) {
	opts.AddLimitedFlags(f)
	f.BoolVarP(&opts.DryRun, "dry-run", "n", false, "do not modify the repository, just print what would be done")
	f.StringVarP(&opts.UnsafeNoSpaceRecovery, "unsafe-recover-no-free-space", "", "", "UNSAFE, READ THE DOCUMENTATION BEFORE USING! Try to recover a repository stuck with no free space. Do not use without trying out 'prune --max-repack-size 0' first.")
	f.Var(&opts.GroupBy, "group-by", "`group` repacked tree blobs by host, paths and/or tags (comma-separated) to reduce client cache churn; should match the clients' `backup --group-by` (default: no grouping)")
}

func (opts *PruneOptions) AddLimitedFlags(f *pflag.FlagSet) {
	var unused bool
	f.StringVar(&opts.MaxUnused, "max-unused", "5%", "tolerate given `limit` of unused data (absolute value in bytes with suffixes k/K, m/M, g/G, t/T, a value in % or the word 'unlimited')")
	f.StringVar(&opts.MaxRepackSize, "max-repack-size", "", "stop after repacking this much data in total (allowed suffixes for `size`: k/K, m/M, g/G, t/T)")
	f.BoolVar(&opts.RepackCacheableOnly, "repack-cacheable-only", false, "only repack packs which are cacheable")
	f.BoolVar(&unused, "repack-small", false, "deprecated. Use --repack-smaller-than to specify a minimum size")
	f.BoolVar(&opts.RepackUncompressed, "repack-uncompressed", false, "repack all uncompressed data")
	f.StringVar(&opts.SmallPackSize, "repack-smaller-than", "", "pack `below-limit` packfiles (allowed suffixes: m/M)")

	err := f.MarkDeprecated("repack-small", "small files are automatically repacked. Use --repack-smaller-than to specify a minimum size")
	if err != nil {
		// MarkDeprecated only returns an error when the flag is not found
		panic(err)
	}
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

	if opts.SmallPackSize != "" {
		size, err := ui.ParseBytes(opts.SmallPackSize)
		if err != nil {
			return errors.Fatalf("invalid number of bytes %q for --repack-smaller-than: %v", opts.SmallPackSize, err)
		} else if size <= 0 {
			return errors.Fatalf("--repack-smaller-than must be larger than zero")
		}
		opts.SmallPackBytes = uint64(size)
	}

	return nil
}

func runPrune(ctx context.Context, opts PruneOptions, gopts global.Options, term ui.Terminal) error {
	err := verifyPruneOptions(&opts)
	if err != nil {
		return err
	}

	if opts.RepackUncompressed && gopts.Compression == repository.CompressionOff {
		return errors.Fatal("disabled compression and `--repack-uncompressed` are mutually exclusive")
	}

	if gopts.NoLock && !opts.DryRun {
		return errors.Fatal("--no-lock is only applicable in combination with --dry-run for prune command")
	}

	printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, term)
	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, opts.DryRun && gopts.NoLock, printer)
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

	return runPruneWithRepo(ctx, opts, gopts, repo, restic.NewIDSet(), printer)
}

func runPruneWithRepo(ctx context.Context, opts PruneOptions, gopts global.Options, repo *repository.Repository, ignoreSnapshots restic.IDSet, printer restic.Printer) error {
	if repo.Cache() == nil && !gopts.JSON {
		printer.S("warning: running prune without a cache, this may be very slow!")
	}

	// loading the index before the snapshots is ok, as we use an exclusive lock here
	err := repo.LoadIndex(ctx, printer)
	if err != nil {
		return err
	}

	popts := repository.PruneOptions{
		DryRun:         opts.DryRun,
		UnsafeRecovery: opts.unsafeRecovery,

		MaxUnusedBytes: opts.maxUnusedBytes,
		MaxRepackBytes: opts.MaxRepackBytes,
		SmallPackBytes: opts.SmallPackBytes,

		RepackCacheableOnly: opts.RepackCacheableOnly,
		RepackUncompressed:  opts.RepackUncompressed,
	}

	plan, err := repository.PlanPrune(ctx, popts, repo, func(ctx context.Context, repo restic.Repository, usedBlobs restic.FindBlobSet) (map[restic.BlobHandle]uint32, error) {
		return getUsedBlobs(ctx, repo, usedBlobs, ignoreSnapshots, opts.GroupBy, printer)
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

	if !gopts.JSON {
		err = printPruneStats(printer, plan.Stats())
		if err != nil {
			return err
		}
	} else {
		gopts.Term.Print(ui.ToJSONString(plan.Stats()))
	}

	// Trigger GC to reset garbage collection threshold
	runtime.GC()

	return plan.Execute(ctx, printer)
}

// printPruneStats prints out the statistics
func printPruneStats(printer restic.Printer, stats repository.PruneStats) error {
	printer.V("\nused:         %10d blobs / %s", stats.Blobs.Used, ui.FormatBytes(stats.Size.Used))
	if stats.Blobs.Duplicate > 0 {
		printer.V("duplicates:   %10d blobs / %s", stats.Blobs.Duplicate, ui.FormatBytes(stats.Size.Duplicate))
	}
	printer.V("unused:       %10d blobs / %s", stats.Blobs.Unused, ui.FormatBytes(stats.Size.Unused))
	if stats.Size.Unref > 0 {
		printer.V("unreferenced:                    %s", ui.FormatBytes(stats.Size.Unref))
	}
	printer.V("total:        %10d blobs / %s", stats.Blobs.Total, ui.FormatBytes(stats.Size.Total))
	printer.V("unused size: %s of total size", ui.FormatPercent(stats.Size.Duplicate+stats.Size.Unused, stats.Size.Total))

	printer.P("\nto repack:    %10d blobs / %s", stats.Blobs.Repack, ui.FormatBytes(stats.Size.Repack))
	printer.P("this removes: %10d blobs / %s", stats.Blobs.Repackrm, ui.FormatBytes(stats.Size.Repackrm))
	printer.P("to delete:    %10d blobs / %s", stats.Blobs.Remove, ui.FormatBytes(stats.Size.Remove+stats.Size.Unref))
	printer.P("total prune:  %10d blobs / %s", stats.Blobs.RemoveTotal, ui.FormatBytes(stats.Size.RemoveTotal))
	if stats.Size.Uncompressed > 0 {
		printer.P("not yet compressed:              %s", ui.FormatBytes(stats.Size.Uncompressed))
	}
	printer.P("remaining:    %10d blobs / %s", stats.Blobs.Remain, ui.FormatBytes(stats.Size.Remain))
	printer.P("unused size after prune: %s (%s of remaining size)",
		ui.FormatBytes(stats.Size.RemainUnused), ui.FormatPercent(stats.Size.RemainUnused, stats.Size.Remain))
	printer.P("")
	printer.V("totally used packs: %10d", stats.Packs.Used)
	printer.V("partly used packs:  %10d", stats.Packs.PartlyUsed)
	printer.V("unused packs:       %10d\n\n", stats.Packs.Unused)

	printer.V("to keep:      %10d packs", stats.Packs.Keep)
	printer.V("to repack:    %10d packs", stats.Packs.Repack)
	printer.V("to delete:    %10d packs", stats.Packs.Remove)
	if stats.Packs.Unref > 0 {
		printer.V("to delete:    %10d unreferenced packs\n\n", stats.Packs.Unref)
	}
	return nil
}

// getUsedBlobs fills usedBlobs with the blobs still in use by non-ignored
// snapshots. When a grouping is requested (a non-empty groupBy), it additionally
// returns a group id per tree blob so that the repack step can cluster tree
// blobs of the same snapshot group into shared pack files. Otherwise it returns
// a nil map and behaves as a classic prune.
func getUsedBlobs(ctx context.Context, repo restic.Repository, usedBlobs restic.FindBlobSet, ignoreSnapshots restic.IDSet, groupBy data.SnapshotGroupByOptions, printer restic.Printer) (map[restic.BlobHandle]uint32, error) {
	grouping := groupBy.Host || groupBy.Path || groupBy.Tag

	var snapshots data.Snapshots
	printer.P("loading all snapshots...")
	err := data.ForAllSnapshots(ctx, repo, repo, ignoreSnapshots,
		func(id restic.ID, sn *data.Snapshot, err error) error {
			if err != nil {
				debug.Log("failed to load snapshot %v (error %v)", id, err)
				return err
			}
			debug.Log("add snapshot %v (tree %v)", id, *sn.Tree)
			snapshots = append(snapshots, sn)
			return nil
		})
	if err != nil {
		return nil, errors.Fatalf("failed loading snapshot: %v", err)
	}

	printer.P("finding data that is still in use for %d snapshots", len(snapshots))

	bar := printer.NewCounter("snapshots")
	bar.SetMax(uint64(len(snapshots)))
	defer bar.Done()

	if !grouping {
		snapshotTrees := make(restic.IDs, 0, len(snapshots))
		for _, sn := range snapshots {
			snapshotTrees = append(snapshotTrees, *sn.Tree)
		}
		if err := data.FindUsedBlobs(ctx, repo, snapshotTrees, usedBlobs, bar); err != nil {
			return nil, errors.Fatalf("failed finding blobs: %v", err)
		}
		return nil, nil
	}

	// Grouped mode: assign each tree blob to a snapshot group so the repack step
	// can cluster tree blobs of the same group into shared pack files. This is a
	// single global walk: groupingSet tags every tree blob with the first group
	// that reaches it, and the shared underlying set makes StreamTrees skip trees
	// already visited by an earlier group, so each tree is walked exactly once.
	// Data blobs pass through untouched (they are not grouped).
	groups, _, err := data.GroupSnapshots(snapshots, groupBy)
	if err != nil {
		return nil, errors.Fatalf("failed grouping snapshots: %v", err)
	}

	// deterministic order of group keys
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// group id 0 is reserved for the shared/fallback bucket in the packer, so
	// real groups start at 1.
	gs := &groupingSet{inner: usedBlobs, treeGroups: make(map[restic.BlobHandle]uint32)}
	for i, k := range keys {
		gs.cur = uint32(i) + 1
		trees := make(restic.IDs, 0, len(groups[k]))
		for _, sn := range groups[k] {
			trees = append(trees, *sn.Tree)
		}
		if err := data.FindUsedBlobs(ctx, repo, trees, gs, bar); err != nil {
			return nil, errors.Fatalf("failed finding blobs: %v", err)
		}
	}
	treeGroups := gs.treeGroups

	// The number of groups we can keep open (and hence localize) is bounded by
	// the file-descriptor limit. When there are no more groups than that, all are
	// localized.
	maxGroups := repository.MaxOpenTreeGroups()
	if len(keys) <= maxGroups {
		printer.P("clustering repacked tree blobs into %d group(s) by %q", len(keys), groupBy.String())
		return treeGroups, nil
	}

	// More groups than we can keep open: keep only the largest ones by tree-blob
	// count (a proxy for metadata footprint, which dominates client cache) and
	// demote the rest to the shared bucket (group 0). No extra walk needed, we
	// already have the per-blob assignment.
	printer.P("grouping by %q yields %d groups, localizing the %d largest (open-file limit); raise `ulimit -n` to localize more",
		groupBy.String(), len(keys), maxGroups)
	demoteSmallGroups(treeGroups, maxGroups)

	return treeGroups, nil
}

// groupRank pairs a group id with its tree-blob count, used to rank groups when
// there are more of them than can be localized at once.
type groupRank struct {
	id uint32
	n  int
}

// demoteSmallGroups keeps only the maxGroups largest groups (by tree-blob count)
// in treeGroups, remapping their ids to a dense 1..maxGroups range, and drops
// every other tree blob so it falls back to the shared bucket (group 0) during
// repacking. It mutates treeGroups in place.
func demoteSmallGroups(treeGroups map[restic.BlobHandle]uint32, maxGroups int) {
	counts := make(map[uint32]int)
	for _, g := range treeGroups {
		counts[g]++
	}
	ranked := make([]groupRank, 0, len(counts))
	for id, n := range counts {
		ranked = append(ranked, groupRank{id: id, n: n})
	}
	// largest first, tie-break by id for determinism
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].n != ranked[j].n {
			return ranked[i].n > ranked[j].n
		}
		return ranked[i].id < ranked[j].id
	})

	// remap the top maxGroups group ids to dense 1..K; ids not in the map are
	// demoted to the shared bucket
	remap := make(map[uint32]uint32, maxGroups)
	for rank, r := range ranked {
		if rank >= maxGroups {
			break
		}
		remap[r.id] = uint32(rank) + 1
	}
	for bh, g := range treeGroups {
		if ng, ok := remap[g]; ok {
			treeGroups[bh] = ng
		} else {
			delete(treeGroups, bh)
		}
	}
}

// groupingSet wraps the used-blob set and records, for every tree blob, the
// first snapshot group (cur) that references it. Data blobs pass through
// untouched. It implements restic.FindBlobSet. It is not safe for concurrent
// use; FindUsedBlobs serializes access with its own lock and the grouped walk
// runs the groups sequentially.
type groupingSet struct {
	inner      restic.FindBlobSet
	treeGroups map[restic.BlobHandle]uint32
	cur        uint32
}

func (s *groupingSet) Has(bh restic.BlobHandle) bool { return s.inner.Has(bh) }

func (s *groupingSet) Insert(bh restic.BlobHandle) {
	if bh.Type == restic.TreeBlob {
		if _, ok := s.treeGroups[bh]; !ok {
			s.treeGroups[bh] = s.cur
		}
	}
	s.inner.Insert(bh)
}
