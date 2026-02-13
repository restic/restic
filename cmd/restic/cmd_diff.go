package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"reflect"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newDiffCommand(globalOptions *global.Options) *cobra.Command {
	var opts DiffOptions

	cmd := &cobra.Command{
		Use:   "diff [flags] snapshotID snapshotID or diff --diff-hosts host1 host2",
		Short: "Show differences between two snapshots",
		Long: `
The "diff" command shows differences from the first to the second snapshot. The
first characters in each line display what has happened to a particular file or
directory:

* +  The item was added
* -  The item was removed
* U  The metadata (access mode, timestamps, ...) for the item was updated
* M  The file's content was modified
* T  The type was changed, e.g. a file was made a symlink
* ?  Bitrot detected: The file's content has changed but all metadata is the same

Metadata comparison will likely not work if a backup was created using the
'--ignore-inode' or '--ignore-ctime' option.

To only compare files in specific subfolders, you can use the "snapshotID:subfolder"
syntax, where "subfolder" is a path within the snapshot.

The --diff-hosts option allows to compare commonalities and differences between
two hosts in the same repository. The comparison can further be filtered by the
use of --path and --tag.


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
			return runDiff(cmd.Context(), opts, *globalOptions, args, globalOptions.Term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// DiffOptions collects all options for the diff command.
type DiffOptions struct {
	ShowMetadata bool
	diffHosts    bool
	data.SnapshotFilter
}

func (opts *DiffOptions) AddFlags(f *pflag.FlagSet) {
	f.BoolVar(&opts.ShowMetadata, "metadata", false, "print changes in metadata")
	f.BoolVar(&opts.diffHosts, "diff-hosts", false, "show differences between 2 hosts in repository")
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, false)
}

func loadSnapshot(ctx context.Context, be restic.Lister, repo restic.LoaderUnpacked, desc string) (*data.Snapshot, string, error) {
	sn, subfolder, err := data.FindSnapshot(ctx, be, repo, desc)
	if err != nil {
		return nil, "", errors.Fatalf("%s", err)
	}
	return sn, subfolder, err
}

// Comparer collects all things needed to compare two snapshots.
type Comparer struct {
	repo        restic.BlobLoader
	opts        DiffOptions
	printChange func(change *Change)
	printError  func(string, ...interface{})
}

type Change struct {
	MessageType string `json:"message_type"` // "change"
	Path        string `json:"path"`
	Modifier    string `json:"modifier"`
}

func NewChange(path string, mode string) *Change {
	return &Change{MessageType: "change", Path: path, Modifier: mode}
}

// DiffStat collects stats for all types of items.
type DiffStat struct {
	Files     int    `json:"files"`
	Dirs      int    `json:"dirs"`
	Others    int    `json:"others"`
	DataBlobs int    `json:"data_blobs"`
	TreeBlobs int    `json:"tree_blobs"`
	Bytes     uint64 `json:"bytes"`
}

// Add adds stats information for node to s.
func (s *DiffStat) Add(node *data.Node) {
	if node == nil {
		return
	}

	switch node.Type {
	case data.NodeTypeFile:
		s.Files++
	case data.NodeTypeDir:
		s.Dirs++
	default:
		s.Others++
	}
}

// addBlobs adds the blobs of node to s.
func addBlobs(bs restic.AssociatedBlobSet, node *data.Node) {
	if node == nil {
		return
	}

	switch node.Type {
	case data.NodeTypeFile:
		for _, blob := range node.Content {
			h := restic.BlobHandle{
				ID:   blob,
				Type: restic.DataBlob,
			}
			bs.Insert(h)
		}
	case data.NodeTypeDir:
		h := restic.BlobHandle{
			ID:   *node.Subtree,
			Type: restic.TreeBlob,
		}
		bs.Insert(h)
	}
}

type DiffStatsContainer struct {
	MessageType                          string                   `json:"message_type"` // "statistics"
	SourceSnapshot                       string                   `json:"source_snapshot"`
	TargetSnapshot                       string                   `json:"target_snapshot"`
	ChangedFiles                         int                      `json:"changed_files"`
	Added                                DiffStat                 `json:"added"`
	Removed                              DiffStat                 `json:"removed"`
	BlobsBefore, BlobsAfter, BlobsCommon restic.AssociatedBlobSet `json:"-"`
}

// updateBlobs updates the blob counters in the stats struct.
func updateBlobs(repo restic.Loader, blobs restic.AssociatedBlobSet, stats *DiffStat, printError func(string, ...interface{})) {
	for h := range blobs.Keys() {
		switch h.Type {
		case restic.DataBlob:
			stats.DataBlobs++
		case restic.TreeBlob:
			stats.TreeBlobs++
		}

		size, found := repo.LookupBlobSize(h.Type, h.ID)
		if !found {
			printError("unable to find blob size for %v", h)
			continue
		}

		stats.Bytes += uint64(size)
	}
}

func (c *Comparer) printDir(ctx context.Context, mode string, stats *DiffStat, blobs restic.AssociatedBlobSet, prefix string, id restic.ID) error {
	debug.Log("print %v tree %v", mode, id)
	tree, err := data.LoadTree(ctx, c.repo, id)
	if err != nil {
		return err
	}

	for item := range tree {
		if item.Error != nil {
			return item.Error
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		node := item.Node
		name := path.Join(prefix, node.Name)
		if node.Type == data.NodeTypeDir {
			name += "/"
		}
		c.printChange(NewChange(name, mode))
		stats.Add(node)
		addBlobs(blobs, node)

		if node.Type == data.NodeTypeDir {
			err := c.printDir(ctx, mode, stats, blobs, name, *node.Subtree)
			if err != nil && err != context.Canceled {
				c.printError("error: %v", err)
			}
		}
	}

	return ctx.Err()
}

func (c *Comparer) collectDir(ctx context.Context, blobs restic.AssociatedBlobSet, id restic.ID) error {
	debug.Log("print tree %v", id)
	tree, err := data.LoadTree(ctx, c.repo, id)
	if err != nil {
		return err
	}

	for item := range tree {
		if item.Error != nil {
			return item.Error
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		node := item.Node
		addBlobs(blobs, node)

		if node.Type == data.NodeTypeDir {
			err := c.collectDir(ctx, blobs, *node.Subtree)
			if err != nil && err != context.Canceled {
				c.printError("error: %v", err)
			}
		}
	}

	return ctx.Err()
}

func (c *Comparer) diffTree(ctx context.Context, stats *DiffStatsContainer, prefix string, id1, id2 restic.ID) error {
	debug.Log("diffing %v to %v", id1, id2)
	tree1, err := data.LoadTree(ctx, c.repo, id1)
	if err != nil {
		return err
	}

	tree2, err := data.LoadTree(ctx, c.repo, id2)
	if err != nil {
		return err
	}

	for dt := range data.DualTreeIterator(tree1, tree2) {
		if dt.Error != nil {
			return dt.Error
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		node1 := dt.Tree1
		node2 := dt.Tree2

		var name string
		if node1 != nil {
			name = node1.Name
		} else {
			name = node2.Name
		}

		addBlobs(stats.BlobsBefore, node1)
		addBlobs(stats.BlobsAfter, node2)

		switch {
		case node1 != nil && node2 != nil:
			name := path.Join(prefix, name)
			mod := ""

			if node1.Type != node2.Type {
				mod += "T"
			}

			if node2.Type == data.NodeTypeDir {
				name += "/"
			}

			if node1.Type == data.NodeTypeFile &&
				node2.Type == data.NodeTypeFile &&
				!reflect.DeepEqual(node1.Content, node2.Content) {
				mod += "M"
				stats.ChangedFiles++

				node1NilContent := *node1
				node2NilContent := *node2
				node1NilContent.Content = nil
				node2NilContent.Content = nil
				// the bitrot detection may not work if `backup --ignore-inode` or `--ignore-ctime` were used
				if node1NilContent.Equals(node2NilContent) {
					// probable bitrot detected
					mod += "?"
				}
			} else if c.opts.ShowMetadata && !node1.Equals(*node2) {
				mod += "U"
			}

			if mod != "" {
				c.printChange(NewChange(name, mod))
			}

			if node1.Type == data.NodeTypeDir && node2.Type == data.NodeTypeDir {
				var err error
				if (*node1.Subtree).Equal(*node2.Subtree) {
					err = c.collectDir(ctx, stats.BlobsCommon, *node1.Subtree)
				} else {
					err = c.diffTree(ctx, stats, name, *node1.Subtree, *node2.Subtree)
				}
				if err != nil && err != context.Canceled {
					c.printError("error: %v", err)
				}
			}
		case node1 != nil && node2 == nil:
			prefix := path.Join(prefix, name)
			if node1.Type == data.NodeTypeDir {
				prefix += "/"
			}
			c.printChange(NewChange(prefix, "-"))
			stats.Removed.Add(node1)

			if node1.Type == data.NodeTypeDir {
				err := c.printDir(ctx, "-", &stats.Removed, stats.BlobsBefore, prefix, *node1.Subtree)
				if err != nil && err != context.Canceled {
					c.printError("error: %v", err)
				}
			}
		case node1 == nil && node2 != nil:
			prefix := path.Join(prefix, name)
			if node2.Type == data.NodeTypeDir {
				prefix += "/"
			}
			c.printChange(NewChange(prefix, "+"))
			stats.Added.Add(node2)

			if node2.Type == data.NodeTypeDir {
				err := c.printDir(ctx, "+", &stats.Added, stats.BlobsAfter, prefix, *node2.Subtree)
				if err != nil && err != context.Canceled {
					c.printError("error: %v", err)
				}
			}
		}
	}

	return ctx.Err()
}

func deleteTreeBlobs(set restic.AssociatedBlobSet) {
	for blob := range set.Keys() {
		if blob.Type == restic.TreeBlob {
			set.Delete(blob)
		}
	}
}

func countAndSizeHosts(repo restic.Repository, set restic.AssociatedBlobSet) (int, uint64) {
	setCount := set.Len()
	size := uint64(0)
	for blob := range set.Keys() {
		bsize, exist := repo.LookupBlobSize(blob.Type, blob.ID)
		if exist {
			size += uint64(bsize)
		}
	}

	return setCount, size
}

func gatherHostData(ctx context.Context, repo restic.Repository, hostname string,
	opts DiffOptions, printer progress.Printer, be restic.Lister,
) (restic.AssociatedBlobSet, int, error) {
	trees := restic.IDs{}

	hostFilter := &data.SnapshotFilter{
		Hosts: []string{hostname},
		Tags:  opts.SnapshotFilter.Tags,
		Paths: opts.SnapshotFilter.Paths,
	}
	for sn := range FindFilteredSnapshots(ctx, be, repo, hostFilter, nil, printer) {
		trees = append(trees, *sn.Tree)
	}

	bar := printer.NewCounter(fmt.Sprintf("snapshots %s", hostname))
	bar.SetMax(uint64(len(trees)))
	blobsHost := repo.NewAssociatedBlobSet()
	if err := data.FindUsedBlobs(ctx, repo, trees, blobsHost, bar); err != nil {
		return nil, 0, err
	}
	bar.Done()

	return blobsHost, len(trees), nil
}

type CountItem struct {
	Hostname      string `json:"hostname,omitempty"`
	SnapshotCount int    `json:"snapshot_count,omitempty"`
	DataBlobCount int    `json:"data_blob_count"`
	DataBlobSize  uint64 `json:"data_blob_size"`
}

type StatDiffHosts struct {
	MessageType string    `json:"message_type"` // "host_differences"
	HostAStats  CountItem `json:"host_A_stats"`
	HostBStats  CountItem `json:"host_B_stats"`
	CommonStats CountItem `json:"common_stats"`
}

// runHostDiff: compare two different host snapshots in the same repository and
// find commonality and differences. Only data blobs are checked for commonality.
// No attempt is made to translate the common data blobs back to pathnames.
func runHostDiff(ctx context.Context, opts DiffOptions, gopts global.Options,
	repo restic.Repository, be restic.Lister,
	hostA string, hostB string, printer progress.Printer,
) error {

	blobsHostA, lenTreesA, err := gatherHostData(ctx, repo, hostA, opts, printer, be)
	if err != nil {
		return err
	}
	blobsHostB, lenTreesB, err := gatherHostData(ctx, repo, hostB, opts, printer, be)
	if err != nil {
		return err
	}

	// remove referenced `tree` blobs
	deleteTreeBlobs(blobsHostA)
	deleteTreeBlobs(blobsHostB)

	// create result sets
	common := blobsHostA.Intersect(blobsHostB)
	onlyA := blobsHostA.Sub(blobsHostB)
	onlyB := blobsHostB.Sub(blobsHostA)

	// count and size
	commonCount, commonSize := countAndSizeHosts(repo, common)
	onlyACount, onlyASize := countAndSizeHosts(repo, onlyA)
	onlyBCount, onlyBSize := countAndSizeHosts(repo, onlyB)

	if !gopts.JSON {
		printer.S("   host A: %s    host B: %s", hostA, hostB)
		printer.S("%7d common data blobs with %12s", commonCount, ui.FormatBytes(commonSize))
		printer.S("%7d only host A blobs with %12s in %5d snapshots", onlyACount, ui.FormatBytes(onlyASize), lenTreesA)
		printer.S("%7d only host B blobs with %12s in %5d snapshots", onlyBCount, ui.FormatBytes(onlyBSize), lenTreesB)
		return nil
	}

	statsDiffHosts := StatDiffHosts{
		MessageType: "host_differences",
		HostAStats:  CountItem{hostA, lenTreesA, onlyACount, onlyASize},
		HostBStats:  CountItem{hostB, lenTreesB, onlyBCount, onlyBSize},
		CommonStats: CountItem{"", 0, commonCount, commonSize},
	}

	err = json.NewEncoder(gopts.Term.OutputWriter()).Encode(statsDiffHosts)
	if err != nil {
		printer.E("JSON encode failed: %v", err)
		return err
	}

	return nil
}

func runDiff(ctx context.Context, opts DiffOptions, gopts global.Options, args []string, term ui.Terminal) error {
	printer := ui.NewProgressPrinter(gopts.JSON, gopts.Verbosity, term)

	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	// cache snapshots listing
	be, err := restic.MemorizeList(ctx, repo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	if err = repo.LoadIndex(ctx, printer); err != nil {
		return err
	}

	if opts.diffHosts {
		if len(args) != 2 {
			return errors.Fatalf("specify two host names")
		}
		return runHostDiff(ctx, opts, gopts, repo, be, args[0], args[1], printer)
	}

	if len(args) != 2 {
		return errors.Fatalf("specify two snapshot IDs")
	}

	sn1, subfolder1, err := loadSnapshot(ctx, be, repo, args[0])
	if err != nil {
		return err
	}

	sn2, subfolder2, err := loadSnapshot(ctx, be, repo, args[1])
	if err != nil {
		return err
	}

	if !gopts.JSON {
		printer.P("comparing snapshot %v to %v:\n\n", sn1.ID().Str(), sn2.ID().Str())
	}

	if sn1.Tree == nil {
		return errors.Errorf("snapshot %v has nil tree", sn1.ID().Str())
	}

	if sn2.Tree == nil {
		return errors.Errorf("snapshot %v has nil tree", sn2.ID().Str())
	}

	sn1.Tree, err = data.FindTreeDirectory(ctx, repo, sn1.Tree, subfolder1)
	if err != nil {
		return err
	}

	sn2.Tree, err = data.FindTreeDirectory(ctx, repo, sn2.Tree, subfolder2)
	if err != nil {
		return err
	}

	c := &Comparer{
		repo:       repo,
		opts:       opts,
		printError: printer.E,
		printChange: func(change *Change) {
			printer.S("%-5s%v", change.Modifier, change.Path)
		},
	}

	if gopts.JSON {
		enc := json.NewEncoder(gopts.Term.OutputWriter())
		c.printChange = func(change *Change) {
			err := enc.Encode(change)
			if err != nil {
				printer.E("JSON encode failed: %v", err)
			}
		}
	}

	if gopts.Quiet {
		c.printChange = func(_ *Change) {}
	}

	stats := &DiffStatsContainer{
		MessageType:    "statistics",
		SourceSnapshot: args[0],
		TargetSnapshot: args[1],
		BlobsBefore:    repo.NewAssociatedBlobSet(),
		BlobsAfter:     repo.NewAssociatedBlobSet(),
		BlobsCommon:    repo.NewAssociatedBlobSet(),
	}
	stats.BlobsBefore.Insert(restic.BlobHandle{Type: restic.TreeBlob, ID: *sn1.Tree})
	stats.BlobsAfter.Insert(restic.BlobHandle{Type: restic.TreeBlob, ID: *sn2.Tree})

	err = c.diffTree(ctx, stats, "/", *sn1.Tree, *sn2.Tree)
	if err != nil {
		return err
	}

	both := stats.BlobsBefore.Intersect(stats.BlobsAfter)
	updateBlobs(repo, stats.BlobsBefore.Sub(both).Sub(stats.BlobsCommon), &stats.Removed, printer.E)
	updateBlobs(repo, stats.BlobsAfter.Sub(both).Sub(stats.BlobsCommon), &stats.Added, printer.E)

	if gopts.JSON {
		err := json.NewEncoder(gopts.Term.OutputWriter()).Encode(stats)
		if err != nil {
			printer.E("JSON encode failed: %v", err)
		}
	} else {
		printer.S("")
		printer.S("Files:       %5d new, %5d removed, %5d changed", stats.Added.Files, stats.Removed.Files, stats.ChangedFiles)
		printer.S("Dirs:        %5d new, %5d removed", stats.Added.Dirs, stats.Removed.Dirs)
		printer.S("Others:      %5d new, %5d removed", stats.Added.Others, stats.Removed.Others)
		printer.S("Data Blobs:  %5d new, %5d removed", stats.Added.DataBlobs, stats.Removed.DataBlobs)
		printer.S("Tree Blobs:  %5d new, %5d removed", stats.Added.TreeBlobs, stats.Removed.TreeBlobs)
		printer.S("  Added:   %-5s", ui.FormatBytes(stats.Added.Bytes))
		printer.S("  Removed: %-5s", ui.FormatBytes(stats.Removed.Bytes))
	}

	return nil
}
