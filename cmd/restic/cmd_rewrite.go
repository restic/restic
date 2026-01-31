package main

import (
	"context"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/restic/restic/internal/walker"
)

func newRewriteCommand(globalOptions *global.Options) *cobra.Command {
	var opts RewriteOptions

	cmd := &cobra.Command{
		Use:   "rewrite [flags] [snapshotID ...]",
		Short: "Rewrite snapshots to exclude unwanted files",
		Long: `
The "rewrite" command excludes files from existing snapshots. It creates new
snapshots containing the same data as the original ones, but without the files
you specify to exclude. All metadata (time, host, tags) will be preserved.

Alternatively you can use one of the --include variants to only include files
in the new snapshot which you want to preserve.

The snapshots to rewrite are specified using the --host, --tag and --path options,
or by providing a list of snapshot IDs. Please note that specifying neither any of
these options nor a snapshot ID will cause the command to rewrite all snapshots.

The special tag 'rewrite' will be added to the new snapshots to distinguish
them from the original ones, unless --forget is used. If the --forget option is
used, the original snapshots will instead be directly removed from the repository.

Please note that the --forget option only removes the snapshots and not the actual
data stored in the repository. In order to delete the no longer referenced data,
use the "prune" command.

When rewrite is used with the --snapshot-summary option, a new snapshot is
created containing statistics summary data. Only two fields in the summary will
be non-zero: TotalFilesProcessed and TotalBytesProcessed.

When rewrite is called with one of the --exclude or --include options,
TotalFilesProcessed and TotalBytesProcessed will be updated in the snapshot summary.

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
			return runRewrite(cmd.Context(), opts, *globalOptions, args, globalOptions.Term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

type snapshotMetadata struct {
	Hostname string
	Time     *time.Time
}

type snapshotMetadataArgs struct {
	Hostname string
	Time     string
}

func (sma snapshotMetadataArgs) empty() bool {
	return sma.Hostname == "" && sma.Time == ""
}

func (sma snapshotMetadataArgs) convert() (*snapshotMetadata, error) {
	if sma.empty() {
		return nil, nil
	}

	var timeStamp *time.Time
	if sma.Time != "" {
		t, err := time.ParseInLocation(global.TimeFormat, sma.Time, time.Local)
		if err != nil {
			return nil, errors.Fatalf("error in time option: %v", err)
		}
		timeStamp = &t
	}
	return &snapshotMetadata{Hostname: sma.Hostname, Time: timeStamp}, nil
}

// RewriteOptions collects all options for the rewrite command.
type RewriteOptions struct {
	Forget          bool
	DryRun          bool
	SnapshotSummary bool

	Metadata snapshotMetadataArgs
	data.SnapshotFilter
	filter.ExcludePatternOptions
	filter.IncludePatternOptions
}

func (opts *RewriteOptions) AddFlags(f *pflag.FlagSet) {
	f.BoolVarP(&opts.Forget, "forget", "", false, "remove original snapshots after creating new ones")
	f.BoolVarP(&opts.DryRun, "dry-run", "n", false, "do not do anything, just print what would be done")
	f.StringVar(&opts.Metadata.Hostname, "new-host", "", "replace hostname")
	f.StringVar(&opts.Metadata.Time, "new-time", "", "replace time of the backup")
	f.BoolVarP(&opts.SnapshotSummary, "snapshot-summary", "s", false, "create snapshot summary record if it does not exist")

	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)
	opts.ExcludePatternOptions.Add(f)
	opts.IncludePatternOptions.Add(f)
}

// rewriteFilterFunc returns the filtered tree ID or an error. If a snapshot summary is returned, the snapshot will
// be updated accordingly.
type rewriteFilterFunc func(ctx context.Context, sn *data.Snapshot, uploader restic.BlobSaver) (restic.ID, *data.SnapshotSummary, error)

func rewriteSnapshot(ctx context.Context, repo *repository.Repository, sn *data.Snapshot, opts RewriteOptions, printer progress.Printer) (bool, error) {
	if sn.Tree == nil {
		return false, errors.Errorf("snapshot %v has nil tree", sn.ID().Str())
	}

	rejectByNameFuncs, err := opts.ExcludePatternOptions.CollectPatterns(printer.E)
	if err != nil {
		return false, err
	}

	includeByNameFuncs, err := opts.IncludePatternOptions.CollectPatterns(printer.E)
	if err != nil {
		return false, err
	}

	metadata, err := opts.Metadata.convert()

	if err != nil {
		return false, err
	}

	condInclude := len(includeByNameFuncs) > 0
	condExclude := len(rejectByNameFuncs) > 0
	var filter rewriteFilterFunc

	if condInclude || condExclude || opts.SnapshotSummary {
		var rewriteNode walker.NodeRewriteFunc
		var keepEmptyDirectoryFunc walker.NodeKeepEmptyDirectoryFunc
		if condInclude {
			rewriteNode, keepEmptyDirectoryFunc = gatherIncludeFilters(includeByNameFuncs, printer)
		} else {
			rewriteNode = gatherExcludeFilters(rejectByNameFuncs, printer)
		}

		rewriter, querySize := walker.NewSnapshotSizeRewriter(rewriteNode, keepEmptyDirectoryFunc)

		filter = func(ctx context.Context, sn *data.Snapshot, uploader restic.BlobSaver) (restic.ID, *data.SnapshotSummary, error) {
			id, err := rewriter.RewriteTree(ctx, repo, uploader, "/", *sn.Tree)
			if err != nil {
				return restic.ID{}, nil, err
			}
			ss := querySize()
			summary := &data.SnapshotSummary{}
			if sn.Summary != nil {
				*summary = *sn.Summary
			}
			summary.TotalFilesProcessed = ss.FileCount
			summary.TotalBytesProcessed = ss.FileSize
			return id, summary, err
		}

	} else {
		filter = func(_ context.Context, sn *data.Snapshot, _ restic.BlobSaver) (restic.ID, *data.SnapshotSummary, error) {
			return *sn.Tree, nil, nil
		}
	}

	return filterAndReplaceSnapshot(ctx, repo, sn,
		filter, opts.DryRun, opts.Forget, metadata, "rewrite", printer, len(includeByNameFuncs) > 0)
}

func filterAndReplaceSnapshot(ctx context.Context, repo restic.Repository, sn *data.Snapshot,
	filter rewriteFilterFunc, dryRun bool, forget bool, newMetadata *snapshotMetadata, addTag string, printer progress.Printer,
	keepEmptySnapshot bool) (bool, error) {

	var filteredTree restic.ID
	var summary *data.SnapshotSummary
	err := repo.WithBlobUploader(ctx, func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		var err error
		filteredTree, summary, err = filter(ctx, sn, uploader)
		return err
	})
	if err != nil {
		return false, err
	}

	if filteredTree.IsNull() {
		if keepEmptySnapshot {
			debug.Log("Snapshot %v not modified", sn)
			return false, nil
		}
		if dryRun {
			printer.P("would delete empty snapshot")
		} else {
			if err = repo.RemoveUnpacked(ctx, restic.WriteableSnapshotFile, *sn.ID()); err != nil {
				return false, err
			}
			debug.Log("removed empty snapshot %v", sn.ID())
			printer.P("removed empty snapshot %v", sn.ID().Str())
		}
		return true, nil
	}

	matchingSummary := true
	if summary != nil {
		matchingSummary = sn.Summary != nil && *summary == *sn.Summary
	}

	if filteredTree == *sn.Tree && newMetadata == nil && matchingSummary {
		debug.Log("Snapshot %v not modified", sn)
		return false, nil
	}

	debug.Log("Snapshot %v modified", sn)
	if dryRun {
		printer.P("would save new snapshot")

		if forget {
			printer.P("would remove old snapshot")
		}

		if newMetadata != nil && newMetadata.Time != nil {
			printer.P("would set time to %s", newMetadata.Time)
		}

		if newMetadata != nil && newMetadata.Hostname != "" {
			printer.P("would set hostname to %s", newMetadata.Hostname)
		}

		return true, nil
	}

	// Always set the original snapshot id as this essentially a new snapshot.
	sn.Original = sn.ID()
	sn.Tree = &filteredTree
	if summary != nil {
		sn.Summary = summary
	}

	if !forget {
		sn.AddTags([]string{addTag})
	}

	if newMetadata != nil && newMetadata.Time != nil {
		printer.P("setting time to %s", *newMetadata.Time)
		sn.Time = *newMetadata.Time
	}

	if newMetadata != nil && newMetadata.Hostname != "" {
		printer.P("setting host to %s", newMetadata.Hostname)
		sn.Hostname = newMetadata.Hostname
	}

	// Save the new snapshot.
	id, err := data.SaveSnapshot(ctx, repo, sn)
	if err != nil {
		return false, err
	}
	printer.P("saved new snapshot %v", id.Str())

	if forget {
		if err = repo.RemoveUnpacked(ctx, restic.WriteableSnapshotFile, *sn.ID()); err != nil {
			return false, err
		}
		debug.Log("removed old snapshot %v", sn.ID())
		printer.P("removed old snapshot %v", sn.ID().Str())
	}
	return true, nil
}

func runRewrite(ctx context.Context, opts RewriteOptions, gopts global.Options, args []string, term ui.Terminal) error {
	hasExcludes := !opts.ExcludePatternOptions.Empty()
	hasIncludes := !opts.IncludePatternOptions.Empty()
	if !opts.SnapshotSummary && !hasExcludes && !hasIncludes && opts.Metadata.empty() {
		return errors.Fatal("Nothing to do: no excludes/includes provided and no new metadata provided")
	} else if hasExcludes && hasIncludes {
		return errors.Fatal("exclude and include patterns are mutually exclusive")
	}

	printer := ui.NewProgressPrinter(false, gopts.Verbosity, term)

	var (
		repo   *repository.Repository
		unlock func()
		err    error
	)

	if opts.Forget {
		printer.P("create exclusive lock for repository")
		ctx, repo, unlock, err = openWithExclusiveLock(ctx, gopts, opts.DryRun, printer)
	} else {
		ctx, repo, unlock, err = openWithAppendLock(ctx, gopts, opts.DryRun, printer)
	}
	if err != nil {
		return err
	}
	defer unlock()

	snapshotLister, err := restic.MemorizeList(ctx, repo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	if err = repo.LoadIndex(ctx, printer); err != nil {
		return err
	}

	changedCount := 0
	for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, &opts.SnapshotFilter, args, printer) {
		printer.P("\n%v", sn)
		changed, err := rewriteSnapshot(ctx, repo, sn, opts, printer)
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

func gatherIncludeFilters(includeByNameFuncs []filter.IncludeByNameFunc, printer progress.Printer) (rewriteNode walker.NodeRewriteFunc, keepEmptyDirectory walker.NodeKeepEmptyDirectoryFunc) {
	inSelectByName := func(nodepath string, node *data.Node) bool {
		for _, include := range includeByNameFuncs {
			matched, childMayMatch := include(nodepath)
			if node.Type == data.NodeTypeDir {
				// include directories if they or some of their children may be included
				if matched || childMayMatch {
					return true
				}
			} else if matched {
				return true
			}
		}
		return false
	}

	rewriteNode = func(node *data.Node, path string) *data.Node {
		if inSelectByName(path, node) {
			if node.Type != data.NodeTypeDir {
				printer.VV("including %q\n", path)
			}
			return node
		}
		return nil
	}

	inSelectByNameDir := func(nodepath string) bool {
		for _, include := range includeByNameFuncs {
			matched, _ := include(nodepath)
			if matched {
				return matched
			}
		}
		return false
	}

	keepEmptyDirectory = func(path string) bool {
		keep := inSelectByNameDir(path)
		if keep {
			printer.VV("including directory %q\n", path)
		}
		return keep
	}

	return rewriteNode, keepEmptyDirectory
}

func gatherExcludeFilters(excludeByNameFuncs []filter.RejectByNameFunc, printer progress.Printer) (rewriteNode walker.NodeRewriteFunc) {
	exSelectByName := func(nodepath string) bool {
		for _, reject := range excludeByNameFuncs {
			if reject(nodepath) {
				return false
			}
		}
		return true
	}

	rewriteNode = func(node *data.Node, path string) *data.Node {
		if exSelectByName(path) {
			return node
		}

		printer.VV("excluding %q\n", path)
		return nil
	}

	return rewriteNode
}
