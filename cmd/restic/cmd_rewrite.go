package main

import (
	"context"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

var cmdRewrite = &cobra.Command{
	Use:   "rewrite [flags] [snapshotID ...]",
	Short: "Rewrite snapshots to exclude unwanted files",
	Long: `
The "rewrite" command excludes files from existing snapshots.
Alternatively you can use rewrite command to include only wanted files and directories.
It creates new snapshots containing the same data as the original ones, but without the files
you specify to exclude. All metadata (time, host, tags) will be preserved.

The snapshots to rewrite are specified using the --host, --tag and --path options,
or by providing a list of snapshot IDs. Please note that specifying neither any of
these options nor a snapshot ID will cause the command to rewrite all snapshots.

The special tag 'rewrite' will be added to the new snapshots to distinguish
them from the original ones, unless --forget is used. If the --forget option is
used, the original snapshots will instead be directly removed from the repository.

Please note that the --forget option only removes the snapshots and not the actual
data stored in the repository. In order to delete the no longer referenced data,
use the "prune" command.

The option --snapshot-summary [-s] creates a new snapshot with snapshot summary data attached.
Only the two fields TotalFilesProcessed and TotalBytesProcessed are non-zero.

For the include option to work more efficiently, it os advisable to use the flag
'--exclude-empty' so only directories needed will be included from the original
snapshot. Otherwise all directories from the original snapshot have to be included.
This however will produce an extra Walk() through the original snapshot tree.

In order to make the include filter work efficiently, an additional read pass through the
directory tree is needed to identify the subdirectories and their parents for the
inclusion of files to work effectively. Otherwise the full directory tree needs to be included
which may contain quite a lot of empty subdirectories. The first read pass
avoids this issue, but it might take a bit more time, depending on the network speed of
the backend storage and the size of the snapshot.

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
		return runRewrite(cmd.Context(), rewriteOptions, globalOptions, args)
	},
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
		t, err := time.ParseInLocation(TimeFormat, sma.Time, time.Local)
		if err != nil {
			return nil, errors.Fatalf("error in time option: %v\n", err)
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
	ExcludeEmptyDir bool

	Metadata snapshotMetadataArgs
	restic.SnapshotFilter
	filter.ExcludePatternOptions
	filter.IncludePatternOptions
}

var rewriteOptions RewriteOptions

func init() {
	cmdRoot.AddCommand(cmdRewrite)

	f := cmdRewrite.Flags()
	f.BoolVarP(&rewriteOptions.Forget, "forget", "", false, "remove original snapshots after creating new ones")
	f.BoolVarP(&rewriteOptions.DryRun, "dry-run", "n", false, "do not do anything, just print what would be done")
	f.StringVar(&rewriteOptions.Metadata.Hostname, "new-host", "", "replace hostname")
	f.StringVar(&rewriteOptions.Metadata.Time, "new-time", "", "replace time of the backup")
	f.BoolVarP(&rewriteOptions.SnapshotSummary, "snapshot-summary", "s", false, "create snapshot summary record if it does not exist")
	f.BoolVarP(&rewriteOptions.ExcludeEmptyDir, "exclude-empty", "X", false, "only for include patterns: exclude empty directories from being created, needs a second walk through the tree")

	initMultiSnapshotFilter(f, &rewriteOptions.SnapshotFilter, true)
	rewriteOptions.ExcludePatternOptions.Add(f)
	rewriteOptions.IncludePatternOptions.Add(f)
}

type rewriteFilterFunc func(ctx context.Context, sn *restic.Snapshot) (restic.ID, error)

type DirectoryNeeded struct {
	node   *restic.Node
	needed bool
}

func rewriteSnapshot(ctx context.Context, repo *repository.Repository, sn *restic.Snapshot, opts RewriteOptions) (bool, error) {
	if sn.Tree == nil {
		return false, errors.Errorf("snapshot %v has nil tree", sn.ID().Str())
	}

	rejectByNameFuncs, err := opts.ExcludePatternOptions.CollectPatterns(Warnf)
	if err != nil {
		return false, err
	}

	includeByNameFuncs, err := opts.IncludePatternOptions.CollectPatterns(Warnf)
	if err != nil {
		return false, err
	}

	metadata, err := opts.Metadata.convert()

	if err != nil {
		return false, err
	}

	// walk the complete snapshot tree and memorize the directory structure
	directoriesNeeded := map[string]DirectoryNeeded{}
	if opts.ExcludeEmptyDir {
		err := walker.Walk(ctx, repo, *sn.Tree, walker.WalkVisitor{ProcessNode: func(parentTreeID restic.ID, nodepath string, node *restic.Node, err error) error {
			if err != nil {
				Printf("Unable to load tree %s\n ... which belongs to snapshot %s - reason %v\n", parentTreeID, sn.ID().Str(), err)
				return walker.ErrSkipNode
			}

			if node == nil {
				return nil
			} else if node.Type == restic.NodeTypeDir {
				directoriesNeeded[nodepath] = DirectoryNeeded{
					node:   node,
					needed: false,
				}
				// filter directories
				for _, include := range includeByNameFuncs {
					matched, childMayMatch := include(nodepath)
					if matched && childMayMatch {
						parentData := directoriesNeeded[nodepath]
						if !parentData.needed { // flip 'needed' bit: off->on
							directoriesNeeded[nodepath] = DirectoryNeeded{
								node:   parentData.node,
								needed: true,
							}
						}
					}
				}
			} else { // include filter processsing - filter file names
				for _, include := range includeByNameFuncs {
					if node.Type == restic.NodeTypeFile {
						matched, childMayMatch := include(nodepath)
						if matched && childMayMatch {
							dirpath := filepath.Dir(nodepath) // parent path
							parentData := directoriesNeeded[dirpath]
							if !parentData.needed { // flip 'needed' bit: off->on
								directoriesNeeded[dirpath] = DirectoryNeeded{
									node:   parentData.node,
									needed: true,
								}
							}
						}
					}
				}
			}
			return nil
		}}) // end walker.Walk

		if err != nil {
			Printf("walker.Walk does not want to run for snapshot %s - reason %v\n", sn.ID().Str(), err)
			return false, err
		}

		// go over all directory structure an find all parent nodes needed
		for { // ever
			more := false
			for dirpath, dirData := range directoriesNeeded {
				if !dirData.needed {
					continue
				}

				parentPath := filepath.Dir(dirpath)
				// TODO: don't know how this is expressed for Windows
				if parentPath == "/" {
					continue
				}

				value := directoriesNeeded[parentPath]
				if value.needed {
					continue
				}

				directoriesNeeded[parentPath] = DirectoryNeeded{
					node:   value.node,
					needed: true,
				}
				more = true
			} // all directories in snapshot

			if !more {
				break
			}
		} // for ever
	} // opts.ExcludeEmptyDir

	var filter rewriteFilterFunc

	if len(rejectByNameFuncs) > 0 {
		selectByName := func(nodepath string) bool {
			for _, reject := range rejectByNameFuncs {
				if reject(nodepath) {
					return false
				}
			}
			return true
		}

		rewriteNode := func(node *restic.Node, path string) *restic.Node {
			if selectByName(path) {
				return node
			}
			Verbosef("excluding %s\n", path)
			return nil
		}

		rewriter, querySize := walker.NewSnapshotSizeRewriter(rewriteNode)

		filter = func(ctx context.Context, sn *restic.Snapshot) (restic.ID, error) {
			id, err := rewriter.RewriteTree(ctx, repo, "/", *sn.Tree)
			if err != nil {
				return restic.ID{}, err
			}
			ss := querySize()
			if sn.Summary == nil { // change of logic: create summary if it wasn't there before
				sn.Summary = &restic.SnapshotSummary{}
			}
			sn.Summary.DataBlobs = ss.DataBlobs
			sn.Summary.TreeBlobs = ss.TreeBlobs
			sn.Summary.TotalFilesProcessed = ss.FileCount
			sn.Summary.TotalBytesProcessed = ss.FileSize
			return id, err
		}

	} else if len(includeByNameFuncs) > 0 {
		selectByName := func(nodepath string, node *restic.Node) bool {
			for _, include := range includeByNameFuncs {
				if node.Type == restic.NodeTypeDir {
					if opts.ExcludeEmptyDir {
						return directoriesNeeded[nodepath].needed
					} else {
						// include directories unconditionally
						return true
					}
				} else if node.Type == restic.NodeTypeFile {
					ifun, childMayMatch := include(nodepath)
					if ifun && childMayMatch {
						return true
					}
				}
			}
			return false
		}

		rewriteNode := func(node *restic.Node, path string) *restic.Node {
			if selectByName(path, node) {
				Verboseff("including %s\n", path)
				return node
			}
			return nil
		}

		rewriter, querySize := walker.NewSnapshotSizeRewriter(rewriteNode)

		filter = func(ctx context.Context, sn *restic.Snapshot) (restic.ID, error) {
			id, err := rewriter.RewriteTree(ctx, repo, "/", *sn.Tree)
			if err != nil {
				return restic.ID{}, err
			}
			ss := querySize()
			if sn.Summary == nil {
				sn.Summary = &restic.SnapshotSummary{}
			}
			sn.Summary.DataBlobs = ss.DataBlobs
			sn.Summary.TreeBlobs = ss.TreeBlobs
			sn.Summary.TotalFilesProcessed = ss.FileCount
			sn.Summary.TotalBytesProcessed = ss.FileSize

			return id, nil
		}

	} else if opts.SnapshotSummary {
		if sn.Summary != nil {
			Printf("snapshot %s has already got snapshot summary data\n", sn.ID().Str())
			return false, nil
		}

		rewriteNode := func(node *restic.Node, path string) *restic.Node {
			return node
		}

		rewriter, querySize := walker.NewSnapshotSizeRewriter(rewriteNode)

		filter = func(ctx context.Context, sn *restic.Snapshot) (restic.ID, error) {
			id, err := rewriter.RewriteTree(ctx, repo, "/", *sn.Tree)
			if err != nil {
				return restic.ID{}, err
			}
			ss := querySize()
			if sn.Summary == nil {
				sn.Summary = &restic.SnapshotSummary{}
			}
			sn.Summary.DataBlobs = ss.DataBlobs
			sn.Summary.TreeBlobs = ss.TreeBlobs
			sn.Summary.TotalFilesProcessed = ss.FileCount
			sn.Summary.TotalBytesProcessed = ss.FileSize
			Verbosef("dataBlobs           %12d\n", ss.DataBlobs)
			Verbosef("treeBlobs           %12d\n", ss.TreeBlobs)
			Verbosef("totalFilesProcessed %12d\n", ss.FileCount)
			Verbosef("totalBytesProcessed %12d\n", ss.FileSize)

			return id, nil
		}

	} else {
		// TODO: question: should metadata modification be changed so that
		// snapshot summary data will always be created??
		filter = func(_ context.Context, sn *restic.Snapshot) (restic.ID, error) {
			return *sn.Tree, nil
		}
	}

	return filterAndReplaceSnapshot(ctx, repo, sn,
		filter, opts.DryRun, opts.Forget, metadata, "rewrite")
}

func filterAndReplaceSnapshot(ctx context.Context, repo restic.Repository, sn *restic.Snapshot,
	filter rewriteFilterFunc, dryRun bool, forget bool, newMetadata *snapshotMetadata, addTag string) (bool, error) {

	wg, wgCtx := errgroup.WithContext(ctx)
	repo.StartPackUploader(wgCtx, wg)

	var filteredTree restic.ID
	wg.Go(func() error {
		var err error
		filteredTree, err = filter(ctx, sn)
		if err != nil {
			return err
		}

		return repo.Flush(wgCtx)
	})
	err := wg.Wait()
	if err != nil {
		return false, err
	}

	if filteredTree.IsNull() {
		if dryRun {
			Verbosef("would delete empty snapshot\n")
		} else {
			if err = repo.RemoveUnpacked(ctx, restic.SnapshotFile, *sn.ID()); err != nil {
				return false, err
			}
			debug.Log("removed empty snapshot %v", sn.ID())
			Verbosef("removed empty snapshot %v\n", sn.ID().Str())
		}
		return true, nil
	}

	if filteredTree == *sn.Tree && newMetadata == nil && sn.Summary == nil {
		debug.Log("Snapshot %v not modified", sn)
		return false, nil
	}

	debug.Log("Snapshot %v modified", sn)
	if dryRun {
		Verbosef("would save new snapshot\n")

		if forget {
			Verbosef("would remove old snapshot\n")
		}

		if newMetadata != nil && newMetadata.Time != nil {
			Verbosef("would set time to %s\n", newMetadata.Time)
		}

		if newMetadata != nil && newMetadata.Hostname != "" {
			Verbosef("would set hostname to %s\n", newMetadata.Hostname)
		}

		return true, nil
	}

	// Always set the original snapshot id as this essentially a new snapshot.
	sn.Original = sn.ID()
	sn.Tree = &filteredTree
	sn.ProgramVersion = version

	if !forget {
		sn.AddTags([]string{addTag})
	}

	if newMetadata != nil && newMetadata.Time != nil {
		Verbosef("setting time to %s\n", *newMetadata.Time)
		sn.Time = *newMetadata.Time
	}

	if newMetadata != nil && newMetadata.Hostname != "" {
		Verbosef("setting host to %s\n", newMetadata.Hostname)
		sn.Hostname = newMetadata.Hostname
	}

	// Save the new snapshot.
	id, err := restic.SaveSnapshot(ctx, repo, sn)
	if err != nil {
		return false, err
	}
	Verbosef("saved new snapshot %v\n", id.Str())

	if forget {
		if err = repo.RemoveUnpacked(ctx, restic.SnapshotFile, *sn.ID()); err != nil {
			return false, err
		}
		debug.Log("removed old snapshot %v", sn.ID())
		Verbosef("removed old snapshot %v\n", sn.ID().Str())
	}
	return true, nil
}

func runRewrite(ctx context.Context, opts RewriteOptions, gopts GlobalOptions, args []string) error {
	exEmpty := opts.ExcludePatternOptions.Empty()
	inEmpty := opts.IncludePatternOptions.Empty()
	if !opts.SnapshotSummary && exEmpty && inEmpty && opts.Metadata.empty() {
		return errors.Fatal("Nothing to do: no includes/excludes provided and no new metadata provided")
	}

	if !exEmpty && !inEmpty {
		return errors.Fatal("You cannot specify include and exclude options simultaneously!")
	}

	if opts.SnapshotSummary && (!exEmpty || !inEmpty) {
		Warnf("option --snapshot-summary is ignored with include/exclude options\n")
	}

	var (
		repo   *repository.Repository
		unlock func()
		err    error
	)

	if opts.Forget {
		Verbosef("create exclusive lock for repository\n")
		ctx, repo, unlock, err = openWithExclusiveLock(ctx, gopts, opts.DryRun)
	} else {
		ctx, repo, unlock, err = openWithAppendLock(ctx, gopts, opts.DryRun)
	}
	if err != nil {
		return err
	}
	defer unlock()

	snapshotLister, err := restic.MemorizeList(ctx, repo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	bar := newIndexProgress(gopts.Quiet, gopts.JSON)
	if err = repo.LoadIndex(ctx, bar); err != nil {
		return err
	}

	changedCount := 0
	for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, &opts.SnapshotFilter, args) {
		Verbosef("\n%v\n", sn)
		changed, err := rewriteSnapshot(ctx, repo, sn, opts)
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

	Verbosef("\n")
	if changedCount == 0 {
		if !opts.DryRun {
			Verbosef("no snapshots were modified\n")
		} else {
			Verbosef("no snapshots would be modified\n")
		}
	} else {
		if !opts.DryRun {
			Verbosef("modified %v snapshots\n", changedCount)
		} else {
			Verbosef("would modify %v snapshots\n", changedCount)
		}
	}

	return nil
}
