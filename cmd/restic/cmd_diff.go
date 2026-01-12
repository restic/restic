package main

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sync/errgroup"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/dump"
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
		Use:   "diff [flags] snapshotID snapshotID",
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

To only compare files in specific subfolders, you can use the
"snapshotID:subfolder" syntax, where "subfolder" is a path within the
snapshot.

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
			return runDiff(cmd.Context(), opts, *globalOptions, args, globalOptions.Term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// DiffOptions collects all options for the diff command.
type DiffOptions struct {
	ShowMetadata    bool
	ShowContentDiff bool
}

func (opts *DiffOptions) AddFlags(f *pflag.FlagSet) {
	f.BoolVar(&opts.ShowMetadata, "metadata", false, "print changes in metadata")
	f.BoolVar(&opts.ShowContentDiff, "content", false, "show content of file differences")
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
	repo         restic.BlobLoader
	opts         DiffOptions
	printChange  func(change *Change)
	printError   func(string, ...interface{})
	contentDiffs []ContentDiff
}

type Change struct {
	MessageType string `json:"message_type"` // "change"
	Path        string `json:"path"`
	Modifier    string `json:"modifier"`
}

type ContentDiff struct {
	node1 *data.Node
	node2 *data.Node
	name  string
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
		case t1 && t2:
			name := filepath.Join(prefix, name)
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
				c.contentDiffs = append(c.contentDiffs, ContentDiff{node1, node2, name})

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
		case t1 && !t2:
			prefix := filepath.Join(prefix, name)
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
		case !t1 && t2:
			prefix := filepath.Join(prefix, name)
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

func runDiff(ctx context.Context, opts DiffOptions, gopts global.Options, args []string, term ui.Terminal) error {
	if len(args) != 2 {
		return errors.Fatalf("specify two snapshot IDs")
	}
	if gopts.JSON && opts.ShowContentDiff {
		return errors.Fatalf("options --JSON and --content are incompatible. Try without --JSON")
	}

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
	if err = repo.LoadIndex(ctx, printer); err != nil {
		return err
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
		contentDiffs: []ContentDiff{},
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

	if opts.ShowContentDiff && len(c.contentDiffs) > 0 {
		if err := createContentsDiffs(ctx, repo, c.contentDiffs, printer, sn1, subfolder1, sn2, subfolder2); err != nil {
			return err
		}
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

// createContentsDiffs runs up to 5 compareWorkers in parallel
func createContentsDiffs(ctx context.Context, repo restic.Repository,
	contentDiffs []ContentDiff, printer progress.Printer,
	sn1 *data.Snapshot, subfolder1 string, sn2 *data.Snapshot, subfolder2 string,
) error {

	var mu sync.Mutex
	chanContent := make(chan ContentDiff)
	wg, ctx := errgroup.WithContext(ctx)

	// send the file comparator data down the channel
	wg.Go(func() error {
		for _, item := range contentDiffs {
			chanContent <- item
		}

		close(chanContent)
		return nil
	})

	const numWorkers = 5
	for i := range numWorkers {
		// activate the workers
		wg.Go(func() error {
			return compareWorker(ctx, repo, chanContent, printer, sn1, subfolder1, sn2, subfolder2, &mu, i+1)
		})
	}

	return wg.Wait()
}

func compareWorker(ctx context.Context, repo restic.Repository,
	chanContent chan ContentDiff, printer progress.Printer,
	sn1 *data.Snapshot, subfolder1 string, sn2 *data.Snapshot, subfolder2 string,
	mu *sync.Mutex, id int,
) error {

	tempDir := os.TempDir()
	for item := range chanContent {
		displayName1 := filepath.Join(subfolder1, item.name)
		displayName2 := filepath.Join(subfolder2, item.name)
		components1 := strings.Split(displayName1, string(os.PathSeparator))
		components2 := strings.Split(displayName2, string(os.PathSeparator))
		one := filepath.Join(tempDir, fmt.Sprintf("%s-%d-%s", sn1.ID().Str(), id, strings.Join(components1, "_")))
		two := filepath.Join(tempDir, fmt.Sprintf("%s-%d-%s", sn2.ID().Str(), id, strings.Join(components2, "_")))

		defer func() {
			_ = os.Remove(one)
			_ = os.Remove(two)
		}()

		// parallel load of the two versions of the same file
		wg, ctx := errgroup.WithContext(ctx)
		wg.Go(func() error {
			return extractFile(ctx, repo, one, item.node1)
		})
		wg.Go(func() error {
			return extractFile(ctx, repo, two, item.node2)
		})

		err := wg.Wait()
		if err != nil {
			return err
		}

		switch runtime.GOOS {
		case "linux", "darwin", "freebsd", "openbsd", "netbsd", "solaris":
			cmd := exec.Command("diff", "-u", one, two)
			stdoutStderr, _ := cmd.CombinedOutput()
			mu.Lock()
			printer.S("\n*** show contents diff for file %q ***", displayName1)
			printer.S("%s\n\n", stdoutStderr)
			mu.Unlock()
		case "windows":
			/*
				cmd := exec.Command("fc", one, two)
				stdoutStderr, _ := cmd.CombinedOutput()
				mu.Lock()
				printer.S("\n*** show contents diff for file %q ***", displayName1)
				printer.S("%s\n\n", stdoutStderr)
				mu.Unlock()
			*/
			mu.Lock()
			printer.S("\n*** show contents diff for file %q ***", displayName1)
			printer.S("no idea of how to run a file comparison successfully without knowing the file tyoe beforehand!")
			mu.Unlock()
		default:
			return errors.Fatalf("don't know how tun run a file difference programme in the %q OS", runtime.GOOS)
		}
	}

	return nil
}

// extractFile extracts the file in node 'node' to 'tempFilename'
// modTime of tempFile is set to Modtime of node.
func extractFile(ctx context.Context, repo restic.Repository, tempFilename string, node *data.Node) error {
	file, err := os.Create(tempFilename)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()

	d := dump.New("", repo, file)
	if err := d.WriteNode(ctx, node); err != nil {
		return err
	}

	// close file, so it gets flushed out of the buffer
	if err := file.Close(); err != nil {
		return err
	}

	// change modTime for tempfile
	return os.Chtimes(tempFilename, node.AccessTime, node.ModTime)
}
