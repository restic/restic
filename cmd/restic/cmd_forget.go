package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/restic/restic/internal/walker"
	"golang.org/x/sync/errgroup"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newForgetCommand(globalOptions *global.Options) *cobra.Command {
	var opts ForgetOptions
	var pruneOpts PruneOptions

	cmd := &cobra.Command{
		Use:   "forget [flags] [snapshot ID] [...]",
		Short: "Remove snapshots from the repository",
		Long: `
The "forget" command removes snapshots according to a policy. All snapshots are
first divided into groups according to "--group-by", and after that the policy
specified by the "--keep-*" options is applied to each group individually.
If there are not enough snapshots to keep one for each duration related
"--keep-{within-,}*" option, the oldest snapshot in the group is kept
additionally.

Please note that this command really only deletes the snapshot object in the
repository, which is a reference to data stored there. In order to remove the
unreferenced data after "forget" was run successfully, see the "prune" command.

Please also read the documentation for "forget" to learn about some important
security considerations.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 3 if there was an error removing one or more snapshots.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
		GroupID:           cmdGroupDefault,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			finalizeSnapshotFilter(&opts.SnapshotFilter)
			return runForget(cmd.Context(), opts, pruneOpts, *globalOptions, globalOptions.Term, args)
		},
	}

	opts.AddFlags(cmd.Flags())
	pruneOpts.AddLimitedFlags(cmd.Flags())
	return cmd
}

type ForgetPolicyCount int

var ErrNegativePolicyCount = errors.New("negative values not allowed, use 'unlimited' instead")
var ErrFailedToRemoveOneOrMoreSnapshots = errors.New("failed to remove one or more snapshots")

func (c *ForgetPolicyCount) Set(s string) error {
	switch s {
	case "unlimited":
		*c = -1
	default:
		val, err := strconv.ParseInt(s, 10, 0)
		if err != nil {
			return err
		}
		if val < 0 {
			return ErrNegativePolicyCount
		}
		*c = ForgetPolicyCount(val)
	}

	return nil
}

func (c *ForgetPolicyCount) String() string {
	switch *c {
	case -1:
		return "unlimited"
	default:
		return strconv.FormatInt(int64(*c), 10)
	}
}

func (c *ForgetPolicyCount) Type() string {
	return "n"
}

// ForgetOptions collects all options for the forget command.
type ForgetOptions struct {
	Last          ForgetPolicyCount
	Hourly        ForgetPolicyCount
	Daily         ForgetPolicyCount
	Weekly        ForgetPolicyCount
	Monthly       ForgetPolicyCount
	Yearly        ForgetPolicyCount
	Within        data.Duration
	WithinHourly  data.Duration
	WithinDaily   data.Duration
	WithinWeekly  data.Duration
	WithinMonthly data.Duration
	WithinYearly  data.Duration
	KeepTags      data.TagLists

	UnsafeAllowRemoveAll bool

	data.SnapshotFilter
	Compact          bool
	ShowRemovedFiles bool
	SearchFiles      bool

	// Grouping
	GroupBy data.SnapshotGroupByOptions
	DryRun  bool
	Prune   bool
}

func (opts *ForgetOptions) AddFlags(f *pflag.FlagSet) {
	f.VarP(&opts.Last, "keep-last", "l", "keep the last `n` snapshots (use 'unlimited' to keep all snapshots)")
	f.VarP(&opts.Hourly, "keep-hourly", "H", "keep the last `n` hourly snapshots (use 'unlimited' to keep all hourly snapshots)")
	f.VarP(&opts.Daily, "keep-daily", "d", "keep the last `n` daily snapshots (use 'unlimited' to keep all daily snapshots)")
	f.VarP(&opts.Weekly, "keep-weekly", "w", "keep the last `n` weekly snapshots (use 'unlimited' to keep all weekly snapshots)")
	f.VarP(&opts.Monthly, "keep-monthly", "m", "keep the last `n` monthly snapshots (use 'unlimited' to keep all monthly snapshots)")
	f.VarP(&opts.Yearly, "keep-yearly", "y", "keep the last `n` yearly snapshots (use 'unlimited' to keep all yearly snapshots)")
	f.VarP(&opts.Within, "keep-within", "", "keep snapshots that are newer than `duration` (eg. 1y5m7d2h) relative to the latest snapshot")
	f.VarP(&opts.WithinHourly, "keep-within-hourly", "", "keep hourly snapshots that are newer than `duration` (eg. 1y5m7d2h) relative to the latest snapshot")
	f.VarP(&opts.WithinDaily, "keep-within-daily", "", "keep daily snapshots that are newer than `duration` (eg. 1y5m7d2h) relative to the latest snapshot")
	f.VarP(&opts.WithinWeekly, "keep-within-weekly", "", "keep weekly snapshots that are newer than `duration` (eg. 1y5m7d2h) relative to the latest snapshot")
	f.VarP(&opts.WithinMonthly, "keep-within-monthly", "", "keep monthly snapshots that are newer than `duration` (eg. 1y5m7d2h) relative to the latest snapshot")
	f.VarP(&opts.WithinYearly, "keep-within-yearly", "", "keep yearly snapshots that are newer than `duration` (eg. 1y5m7d2h) relative to the latest snapshot")
	f.Var(&opts.KeepTags, "keep-tag", "keep snapshots with this `taglist` (can be specified multiple times)")
	f.BoolVar(&opts.UnsafeAllowRemoveAll, "unsafe-allow-remove-all", false, "allow deleting all snapshots of a snapshot group")
	f.BoolVar(&opts.ShowRemovedFiles, "show-removed-files", false, "show files which would be removed")
	f.BoolVar(&opts.SearchFiles, "search-files", false, "search for identically named files and exclude")

	f.StringArrayVar(&opts.Hosts, "hostname", nil, "only consider snapshots with the given `hostname` (can be specified multiple times)")
	err := f.MarkDeprecated("hostname", "use --host")
	if err != nil {
		// MarkDeprecated only returns an error when the flag is not found
		panic(err)
	}
	// must be defined after `--hostname` to not override the default value from the environment
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, false)

	f.BoolVarP(&opts.Compact, "compact", "c", false, "use compact output format")
	opts.GroupBy = data.SnapshotGroupByOptions{Host: true, Path: true}
	f.VarP(&opts.GroupBy, "group-by", "g", "`group` snapshots by host, paths and/or tags, separated by comma (disable grouping with '')")
	f.BoolVarP(&opts.DryRun, "dry-run", "n", false, "do not delete anything, just print what would be done")
	f.BoolVar(&opts.Prune, "prune", false, "automatically run the 'prune' command if snapshots have been removed")

	f.SortFlags = false
}

func verifyForgetOptions(opts *ForgetOptions) error {
	if opts.ShowRemovedFiles && !opts.DryRun {
		return errors.Fatal("option --show-removed-files needs option --dry-run")

	}
	if opts.SearchFiles && !opts.ShowRemovedFiles {
		return errors.Fatal("option --search-files needs option --show-removed-files")
	}

	if opts.Last < -1 || opts.Hourly < -1 || opts.Daily < -1 || opts.Weekly < -1 ||
		opts.Monthly < -1 || opts.Yearly < -1 {
		return errors.Fatal("negative values other than -1 are not allowed for --keep-*")
	}

	for _, d := range []data.Duration{opts.Within, opts.WithinHourly, opts.WithinDaily,
		opts.WithinMonthly, opts.WithinWeekly, opts.WithinYearly} {
		if d.Hours < 0 || d.Days < 0 || d.Months < 0 || d.Years < 0 {
			return errors.Fatal("durations containing negative values are not allowed for --keep-within*")
		}
	}

	return nil
}

func runForget(ctx context.Context, opts ForgetOptions, pruneOptions PruneOptions, gopts global.Options, term ui.Terminal, args []string) error {
	err := verifyForgetOptions(&opts)
	if err != nil {
		return err
	}

	err = verifyPruneOptions(&pruneOptions)
	if err != nil {
		return err
	}

	if gopts.NoLock && !opts.DryRun {
		return errors.Fatal("--no-lock is only applicable in combination with --dry-run for forget command")
	}

	printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, term)
	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, opts.DryRun && gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	snapshotLister, err := restic.MemorizeList(ctx, repo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	var snapshots data.Snapshots
	removeSnIDs := restic.NewIDSet()

	for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, &opts.SnapshotFilter, args, printer) {
		snapshots = append(snapshots, sn)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	var jsonGroups []*ForgetGroup

	if len(args) > 0 {
		// When explicit snapshots args are given, remove them immediately.
		for _, sn := range snapshots {
			removeSnIDs.Insert(*sn.ID())
		}
	} else {
		snapshotGroups, _, err := data.GroupSnapshots(snapshots, opts.GroupBy)
		if err != nil {
			return err
		}

		policy := data.ExpirePolicy{
			Last:          int(opts.Last),
			Hourly:        int(opts.Hourly),
			Daily:         int(opts.Daily),
			Weekly:        int(opts.Weekly),
			Monthly:       int(opts.Monthly),
			Yearly:        int(opts.Yearly),
			Within:        opts.Within,
			WithinHourly:  opts.WithinHourly,
			WithinDaily:   opts.WithinDaily,
			WithinWeekly:  opts.WithinWeekly,
			WithinMonthly: opts.WithinMonthly,
			WithinYearly:  opts.WithinYearly,
			Tags:          opts.KeepTags,
		}

		if policy.Empty() {
			if opts.UnsafeAllowRemoveAll {
				if opts.SnapshotFilter.Empty() {
					return errors.Fatal("--unsafe-allow-remove-all is not allowed unless a snapshot filter option is specified")
				}
				// UnsafeAllowRemoveAll together with snapshot filter is fine
			} else {
				return errors.Fatal("no policy was specified, no snapshots will be removed")
			}
		}

		printer.P("Applying Policy: %v\n", policy)

		for k, snapshotGroup := range snapshotGroups {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			if gopts.Verbose >= 1 && !gopts.JSON {
				err = PrintSnapshotGroupHeader(gopts.Term.OutputWriter(), k)
				if err != nil {
					return err
				}
			}

			var key data.SnapshotGroupKey
			if json.Unmarshal([]byte(k), &key) != nil {
				return err
			}

			var fg ForgetGroup
			fg.Tags = key.Tags
			fg.Host = key.Hostname
			fg.Paths = key.Paths

			keep, remove, reasons := data.ApplyPolicy(snapshotGroup, policy)

			if !policy.Empty() && len(keep) == 0 {
				return fmt.Errorf("refusing to delete last snapshot of snapshot group \"%v\"", key.String())
			}
			if len(keep) != 0 && !gopts.Quiet && !gopts.JSON {
				printer.P("keep %d snapshots:\n", len(keep))
				if err := PrintSnapshots(gopts.Term.OutputWriter(), keep, reasons, opts.Compact); err != nil {
					return err
				}
				printer.P("\n")
			}
			fg.Keep = asJSONSnapshots(keep)

			if len(remove) != 0 && !gopts.Quiet && !gopts.JSON {
				printer.P("remove %d snapshots:\n", len(remove))
				if err := PrintSnapshots(gopts.Term.OutputWriter(), remove, nil, opts.Compact); err != nil {
					return err
				}
				printer.P("\n")
			}
			fg.Remove = asJSONSnapshots(remove)

			fg.Reasons = asJSONKeeps(reasons)

			jsonGroups = append(jsonGroups, &fg)

			for _, sn := range remove {
				removeSnIDs.Insert(*sn.ID())
			}
		}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	if opts.ShowRemovedFiles {
		if err := showRemovedFiles(ctx, repo, removeSnIDs, opts, gopts, snapshotLister, printer); err != nil {
			return err
		}
	}

	// these are the snapshots that failed to be removed
	failedSnIDs := restic.NewIDSet()
	if len(removeSnIDs) > 0 {
		if !opts.DryRun {
			bar := printer.NewCounter("files deleted")
			err := restic.ParallelRemove(ctx, repo, removeSnIDs, restic.WriteableSnapshotFile, func(id restic.ID, err error) error {
				if err != nil {
					printer.E("unable to remove %v/%v from the repository\n", restic.SnapshotFile, id)
					failedSnIDs.Insert(id)
				} else {
					printer.VV("removed %v/%v\n", restic.SnapshotFile, id)
				}
				return nil
			}, bar)
			bar.Done()
			if err != nil {
				return err
			}
		} else {
			printer.P("Would have removed the following snapshots:\n%v\n\n", removeSnIDs)
		}
	}

	if gopts.JSON && len(jsonGroups) > 0 {
		err = printJSONForget(gopts.Term.OutputWriter(), jsonGroups)
		if err != nil {
			return err
		}
	}

	if len(failedSnIDs) > 0 {
		return ErrFailedToRemoveOneOrMoreSnapshots
	}

	if len(removeSnIDs) > 0 && opts.Prune {
		if opts.DryRun {
			printer.P("%d snapshots would be removed, running prune dry run\n", len(removeSnIDs))
		} else {
			printer.P("%d snapshots have been removed, running prune\n", len(removeSnIDs))
		}
		pruneOptions.DryRun = opts.DryRun
		return runPruneWithRepo(ctx, pruneOptions, repo, removeSnIDs, printer)
	}

	return nil
}

// ForgetGroup helps to print what is forgotten in JSON.
type ForgetGroup struct {
	Tags    []string     `json:"tags"`
	Host    string       `json:"host"`
	Paths   []string     `json:"paths"`
	Keep    []Snapshot   `json:"keep"`
	Remove  []Snapshot   `json:"remove"`
	Reasons []KeepReason `json:"reasons"`
}

func asJSONSnapshots(list data.Snapshots) []Snapshot {
	var resultList []Snapshot
	for _, sn := range list {
		k := Snapshot{
			Snapshot: sn,
			ID:       sn.ID(),
			ShortID:  sn.ID().Str(),
		}
		resultList = append(resultList, k)
	}
	return resultList
}

// KeepReason helps to print KeepReasons as JSON with Snapshots with their ID included.
type KeepReason struct {
	Snapshot Snapshot `json:"snapshot"`
	Matches  []string `json:"matches"`
}

func asJSONKeeps(list []data.KeepReason) []KeepReason {
	var resultList []KeepReason
	for _, keep := range list {
		k := KeepReason{
			Snapshot: Snapshot{
				Snapshot: keep.Snapshot,
				ID:       keep.Snapshot.ID(),
				ShortID:  keep.Snapshot.ID().Str(),
			},
			Matches: keep.Matches,
		}
		resultList = append(resultList, k)
	}
	return resultList
}

func printJSONForget(stdout io.Writer, forgets []*ForgetGroup) error {
	return json.NewEncoder(stdout).Encode(forgets)
}

/*==============================================================================
 *
 * show files which are about to be removed / forgotten
 *
 *==============================================================================

	calling diagram:

	showRemovedFiles
		FindUsedBlobs            // find used blobs
		removeStillUsedBlobs
			StreamTrees            // find out if blobs are still in use by other snapshots
		createDeletedFilenames
			walker.Walk            // relate blobs to snapshot and filenames, build 'filesToDelete'
			processOtherPathnames  // used by option --search-files,
				StreamTrees          // filter out other filenames still in use
			generateJSONData
			print result           // text and JSON output
*/

type subNode struct {
	ID   restic.ID
	node *data.Node
}

type subNodeSnap struct {
	node     *data.Node
	snapshot *data.Snapshot
}

type DeleteFileInfo struct {
	SnapshotID restic.ID `json:"snapshot"`
	Path       string    `json:"path"`
	Mtime      time.Time `json:"mtime"`
	Size       uint64    `json:"size"`
}

type DeletedFilenamesJSON struct {
	MessageType  string           `json:"message_type"` // always "deleted_files"
	DeletedFiles []DeleteFileInfo `json:"files"`
}

type ShowRemoved struct {
	selectedSnapshots  []*data.Snapshot
	selectedTrees      []restic.ID
	allOtherTrees      []restic.ID
	otherParentToChild map[restic.ID][]subNode
	searchFiles        bool
	printer            progress.Printer
}

// makeShowRemoved: initializes &ShowRemoved
func makeShowRemoved(searchFiles bool, printer progress.Printer) *ShowRemoved {
	return &ShowRemoved{
		selectedSnapshots:  []*data.Snapshot{},
		selectedTrees:      []restic.ID{},
		allOtherTrees:      []restic.ID{},
		otherParentToChild: make(map[restic.ID][]subNode),
		searchFiles:        searchFiles,
		printer:            printer,
	}
}

// removeStillUsedBlobs looks in all other snapshots for blobs which are still
// in use and removes them from 'uniqueBlobs'
// at the same time, the tree hierarchy is collected for the 'allOtherTrees'
func (sr *ShowRemoved) removeStillUsedBlobs(ctx context.Context, repo restic.Repository,
	uniqueBlobs restic.AssociatedBlobSet,
) error {
	var lock sync.Mutex
	sr.printer.P("find still used blobs ...")
	bar := sr.printer.NewCounter("all other snapshots")
	defer bar.Done()
	seenTree := restic.NewIDSet()
	err := data.StreamTrees(ctx, repo, sr.allOtherTrees, bar, func(tree restic.ID) bool {
		lock.Lock()
		seen := seenTree.Has(tree)
		seenTree.Insert(tree)
		uniqueBlobs.Delete(restic.BlobHandle{ID: tree, Type: restic.TreeBlob})
		lock.Unlock()
		return seen
	}, func(id restic.ID, err error, nodes data.TreeNodeIterator) error {
		if err != nil {
			return fmt.Errorf("LoadTree(%v) returned error %v", id.Str(), err)
		}

		children := []subNode{}
		for tree := range nodes {
			if tree.Error != nil {
				return fmt.Errorf("LoadTree returned error %v", tree.Error)
			}
			node := tree.Node
			switch node.Type {
			case data.NodeTypeFile:
				for _, blob := range node.Content {
					lock.Lock()
					uniqueBlobs.Delete(restic.BlobHandle{ID: blob, Type: restic.DataBlob})
					lock.Unlock()
				}
			case data.NodeTypeDir:
				if sr.searchFiles {
					children = append(children, subNode{*node.Subtree, node})
				}
			}
		}
		if sr.searchFiles {
			lock.Lock()
			sr.otherParentToChild[id] = children
			lock.Unlock()
		}
		return nil
	})

	return err
}

// processOtherPathnames is activated when option --search-files is called for
// search through all the trees attached to 'sr.allOtherTrees'
func (sr *ShowRemoved) processOtherPathnames(ctx context.Context, repo restic.Repository,
	filesToDelete map[string]map[subNode]subNodeSnap, printer progress.Printer,
) error {
	// build tree topology for all other snapshots
	otherDirectoryTimes := makeDirectoryTree(sr.allOtherTrees, sr.otherParentToChild)

	printer.P("look for identical pathnames ...")
	seenTrees := restic.NewIDSet()
	var lock sync.Mutex
	bar := sr.printer.NewCounter("all other snapshots")
	defer bar.Done()
	err := data.StreamTrees(ctx, repo, sr.allOtherTrees, bar, func(tree restic.ID) bool {
		seen := seenTrees.Has(tree)
		seenTrees.Insert(tree)
		return seen
	}, func(parent restic.ID, err error, nodes data.TreeNodeIterator) error {
		if err != nil {
			return fmt.Errorf("LoadTree(%v) returned error %v", parent.Str(), err)
		}

		otherPath, ok := otherDirectoryTimes[parent]
		if !ok {
			return nil
		}

		for tree := range nodes {
			if tree.Error != nil {
				return fmt.Errorf("LoadTree returned error %v", tree.Error)
			}
			if tree.Node.Type != data.NodeTypeFile {
				continue
			}
			lock.Lock()
			delete(filesToDelete, filepath.Join(otherPath, tree.Node.Name))
			lock.Unlock()
		}
		return nil
	})

	return err
}

// createDeletedFilenames walks through the selected snapshots (treeList)
// and takes note of the blobs in 'uniqueBlobs'
// the tree IDs related to these blobs are collected for naming and finding the
// oldest snapshot
func (sr *ShowRemoved) createDeletedFilenames(ctx context.Context, repo restic.Repository,
	uniqueBlobs restic.AssociatedBlobSet, gopts global.Options, printer progress.Printer,
) error {

	printer.P("build file list to be deleted ...")
	filesToDelete := make(map[string]map[subNode]subNodeSnap)
	now := time.Now()
	if err := walkParallel(ctx, repo, sr.selectedSnapshots, uniqueBlobs, filesToDelete, printer); err != nil {
		return err
	}
	printer.P("file list built")
	printer.VV("time to build delete list %.1f seconds", time.Since(now).Seconds())

	if sr.searchFiles {
		// match pathnames from 'allOtherTrees' and remove from 'filesToDelete'
		now = time.Now()
		if err := sr.processOtherPathnames(ctx, repo, filesToDelete, printer); err != nil {
			return err
		}
		printer.VV("time to find identical pathnames %.1f seconds", time.Since(now).Seconds())
	}

	// convert 'filesToDelete' into deletedFilenamesJSON.DeletedFiles
	now = time.Now()
	deletedFilenamesJSON, err := sr.generateJSONData(filesToDelete)
	if err != nil {
		return err
	}
	printer.VV("time to generate output %.1f seconds", time.Since(now).Seconds())

	if !gopts.JSON {
		printer.P("\n*** files to be removed ***")
		for _, item := range deletedFilenamesJSON.DeletedFiles {
			printer.P("%s %12s %v %s", item.SnapshotID.Str(), ui.FormatBytes(item.Size), item.Mtime.Format(time.DateTime), item.Path)
		}
		return nil
	}

	return json.NewEncoder(gopts.Term.OutputWriter()).Encode(deletedFilenamesJSON)
}

// generateJSONData collects data blobs from 'filesToDelete'
// The structure for JSON is created and filled.
func (sr *ShowRemoved) generateJSONData(filesToDelete map[string]map[subNode]subNodeSnap) (*DeletedFilenamesJSON, error) {

	resultJSON := &DeletedFilenamesJSON{
		MessageType:  "deleted_files",
		DeletedFiles: make([]DeleteFileInfo, 0, len(filesToDelete)),
	}

	for _, name := range slices.Sorted(maps.Keys(filesToDelete)) {
		oldest := slices.MinFunc(slices.Collect(maps.Values(filesToDelete[name])), func(a, b subNodeSnap) int {
			return a.snapshot.Time.Compare(b.snapshot.Time)
		})

		newEntry := DeleteFileInfo{
			Path:       name,
			Size:       oldest.node.Size,
			Mtime:      oldest.node.ModTime.Truncate(time.Second),
			SnapshotID: *(oldest.snapshot).ID(),
		}
		resultJSON.DeletedFiles = append(resultJSON.DeletedFiles, newEntry)
	}

	return resultJSON, nil
}

// showRemovedFiles prepares a list of files which are going to be removed
// when forget --prune is run for 'removeSnIDs'
// this function is the main driver
func showRemovedFiles(ctx context.Context, repo restic.Repository,
	removeSnIDs restic.IDSet, opts ForgetOptions,
	gopts global.Options, snapshotLister restic.Lister, printer progress.Printer,
) error {
	if err := repo.LoadIndex(ctx, printer); err != nil {
		return err
	}

	sr := makeShowRemoved(opts.SearchFiles, printer)
	for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, &data.SnapshotFilter{}, nil, printer) {
		if removeSnIDs.Has(*sn.ID()) {
			sr.selectedTrees = append(sr.selectedTrees, *sn.Tree)
			sr.selectedSnapshots = append(sr.selectedSnapshots, sn)
		} else {
			sr.allOtherTrees = append(sr.allOtherTrees, *sn.Tree)
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	now := time.Now()
	uniqueBlobs := repo.NewAssociatedBlobSet()
	if err := data.FindUsedBlobs(ctx, repo, sr.selectedTrees, uniqueBlobs, nil); err != nil {
		return err
	}
	printer.VV("time to gather used blobs %.1f seconds", time.Since(now).Seconds())

	now = time.Now()
	if err := sr.removeStillUsedBlobs(ctx, repo, uniqueBlobs); err != nil {
		return err
	}
	printer.VV("time to remove still used blobs %.1f seconds", time.Since(now).Seconds())
	return sr.createDeletedFilenames(ctx, repo, uniqueBlobs, gopts, printer)
}

// makeDirectoryTree maps a tuple 'subNode' to a treeID and a pathname
// the mapping from parent to pathname is unique, but the reverse is certainly not!
func makeDirectoryTree(treeRoots []restic.ID, parentToChild map[restic.ID][]subNode,
) (directoryNames map[restic.ID]string) {

	directoryNames = make(map[restic.ID]string)
	// build entries for all tree roots
	for _, root := range treeRoots {
		directoryNames[root] = "/"
	}

	// iteratively fill in directoryNames (breadth first search)
	seen := restic.NewIDSet()
	for changed := true; changed; {
		changed = false
		for parent, children := range parentToChild {
			parentPath, ok := directoryNames[parent]
			if !ok || seen.Has(parent) {
				continue
			}
			for _, item := range children {
				if _, ok := directoryNames[item.ID]; !ok {
					directoryNames[item.ID] = filepath.Join(parentPath, item.node.Name)
					changed = true
				}
			}
			seen.Insert(parent)
		}
	}

	return directoryNames
}

// walkParallel walks all the snapshoots in selectedSnapshots in parallel
// it generates the delete file list from the blobs in 'uniqueBlobs'
func walkParallel(ctx context.Context, repo restic.Repository, selectedSnapshots []*data.Snapshot,
	uniqueBlobs restic.AssociatedBlobSet, filesToDelete map[string]map[subNode]subNodeSnap,
	printer progress.Printer,
) error {

	var lock sync.Mutex
	chanSnapshot := make(chan *data.Snapshot)
	wg, wgCtx := errgroup.WithContext(ctx)

	// go routine 1: dispense snapshots
	wg.Go(func() error {
		for _, sn := range selectedSnapshots {
			chanSnapshot <- sn
		}

		close(chanSnapshot)
		return nil
	})

	worker := func() error {
		for sn := range chanSnapshot {
			err := walker.Walk(wgCtx, repo, *sn.Tree, walker.WalkVisitor{
				ProcessNode: func(parentTreeID restic.ID, pathname string, node *data.Node, nodeErr error) error {
					if nodeErr != nil {
						printer.E("Unable to load tree %s\n ... which belongs to snapshot %s - reason %v\n",
							parentTreeID.Str(), sn.ID().Str(), nodeErr)
						return nodeErr
					}
					if node == nil {
						return nil
					}

					if node.Type == data.NodeTypeFile {
						fixedNode := subNode{ID: parentTreeID, node: node}
						for _, blob := range node.Content {
							if !uniqueBlobs.Has(restic.BlobHandle{ID: blob, Type: restic.DataBlob}) {
								continue
							}

							lock.Lock()
							if _, ok := filesToDelete[pathname]; !ok {
								filesToDelete[pathname] = make(map[subNode]subNodeSnap)
							}
							if _, ok := filesToDelete[pathname][fixedNode]; !ok {
								filesToDelete[pathname][fixedNode] = subNodeSnap{
									node:     node,
									snapshot: sn,
								}
							}
							lock.Unlock()

							// first blob is enough to construct a complete entry
							break
						}
					}

					return nil
				}})
			if err != nil {
				return err
			}
		}

		return nil
	}

	// go routine 2 .. n+1: workers
	for i := 0; i < runtime.GOMAXPROCS(0); i++ {
		wg.Go(worker)
	}

	return wg.Wait()
}
