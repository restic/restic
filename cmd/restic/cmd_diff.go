package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sync/errgroup"
	"path/filepath"
	"reflect"
	"sort"
	"sync"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
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
	diffSizeMax     string
	diffSizeBytes   uint64
}

func (opts *DiffOptions) AddFlags(f *pflag.FlagSet) {
	f.BoolVar(&opts.ShowMetadata, "metadata", false, "print changes in metadata")
	f.BoolVar(&opts.ShowContentDiff, "content", false, "show content of file differences for text files")
	f.StringVar(&opts.diffSizeMax, "diff-max-size", "", "limit compare `size` (allowed suffixes: k/K, m/M), default 1MiB")
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

	opts.diffSizeBytes = uint64(1 << 20) // 1 MiB
	if opts.diffSizeMax != "" {
		size, err := ui.ParseBytes(opts.diffSizeMax)
		if err != nil {
			return errors.Fatalf("invalid number of bytes %q for --diff-max-size: %v", opts.diffSizeMax, err)
		}
		opts.diffSizeBytes = uint64(size)
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
		if err := createContentsDiffs(ctx, repo, c.contentDiffs, printer, sn1, subfolder1,
			sn2, subfolder2, opts.diffSizeBytes); err != nil {
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
	diffSizeBytes uint64,
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
	for range numWorkers {
		// activate the workers
		wg.Go(func() error {
			return compareWorker(ctx, repo, chanContent, printer, sn1, subfolder1, sn2, subfolder2, &mu, diffSizeBytes)
		})
	}

	return wg.Wait()
}

func compareWorker(ctx context.Context, repo restic.Repository,
	chanContent chan ContentDiff, printer progress.Printer,
	sn1 *data.Snapshot, subfolder1 string, sn2 *data.Snapshot, subfolder2 string,
	mu *sync.Mutex, diffSizeBytes uint64,
) error {
	for item := range chanContent {
		displayName1 := filepath.Join(subfolder1, item.name)
		displayName2 := filepath.Join(subfolder2, item.name)
		one := fmt.Sprintf("%s %s %v", sn1.ID().Str(), displayName1, item.node1.ModTime)
		two := fmt.Sprintf("%s %s %v", sn2.ID().Str(), displayName2, item.node2.ModTime)

		var isBinFile1, isBinFile2, oversized1, oversized2 bool
		var buf1, buf2 []byte

		// parallel load of the two versions of the same file
		wg, ctx := errgroup.WithContext(ctx)
		wg.Go(func() error {
			var err error
			isBinFile1, buf1, oversized1, err = extractFile(ctx, repo, item.node1, diffSizeBytes)
			return err
		})

		wg.Go(func() error {
			var err error
			isBinFile2, buf2, oversized2, err = extractFile(ctx, repo, item.node2, diffSizeBytes)
			return err
		})

		err := wg.Wait()
		if err != nil {
			return err
		}

		if isBinFile1 || isBinFile2 {
			mu.Lock()
			printer.P("--- %s", one)
			printer.P("+++ %s", two)
			printer.P("file %s is a binary file and the two file differ", displayName1)
			mu.Unlock()
		} else {
			// compute the edits using the Myers diff algorithm as suggested by Gemini
			edits := myers.ComputeEdits(span.URIFromPath(item.name), string(buf1), string(buf2))
			// generate the Unified Diff format
			diff := gotextdiff.ToUnified(one, two, string(buf1), edits)
			mu.Lock()
			printer.P("\n%s", diff)
			if oversized1 || oversized2 {
				printer.P("\n*** file has been truncated; there will be artfacts in the comparison output ***")
			}
			mu.Unlock()
		}
	}

	return nil
}

func extractShortFile(ctx context.Context, repo restic.Repository, node *data.Node) (bool, []byte, bool, error) {
	var fileBuf bytes.Buffer
	d := dump.New("", repo, &fileBuf)
	if err := d.WriteNode(ctx, node); err != nil {
		return false, nil, false, err
	}

	// search the first up to 1024 bytes to check if it is a binary file
	out := fileBuf.Bytes()
	length := min(1024, len(out))
	isBinaryFile := isBinary(out[:length])

	return isBinaryFile, out, false, nil
}

func checkBinaryFile(ctx context.Context, repo restic.Repository, node *data.Node) (bool, error) {
	// decode blob
	var fileBuf bytes.Buffer
	d := dump.New("", repo, &fileBuf)
	if err := d.WriteNode(ctx, node); err != nil {
		return false, err
	}

	// check for binary data and get out quickly if it is a binary file
	out := fileBuf.Bytes()
	length := min(1024, len(out))
	isBinaryFile := isBinary(out[:length])
	return isBinaryFile, nil
}

func assembleShorterFile(repo restic.Repository, tempNode *data.Node, node *data.Node, diffSizeBytes uint64) {
	// assemble a shorter file with a size just above the cutoff limit
	dataBlobs := make([]restic.ID, 0, len(node.Content))
	currentSize := uint64(0)
	for _, blobID := range node.Content {
		size, exists := repo.LookupBlobSize(restic.DataBlob, blobID)
		if exists {
			dataBlobs = append(dataBlobs, blobID)
			currentSize += uint64(size)
		}
		if currentSize > diffSizeBytes {
			break
		}
	}
	tempNode.Content = dataBlobs[:]
}

// extractFile extracts node 'node' into a buffer, limit buffer to 'diffSizeBytes'
func extractFile(ctx context.Context, repo restic.Repository, node *data.Node, diffSizeBytes uint64) (bool, []byte, bool, error) {

	if node.Size <= diffSizeBytes {
		return extractShortFile(ctx, repo, node)
	}

	// logic to deal with large files
	// 1. create a new temporary node so we can present a shorter file
	tempNode := &data.Node{}
	err := DeepCopyJSON(node, tempNode)
	if err != nil {
		return false, nil, false, err
	}

	// 2. check for binray file in first blob
	tempNode.Content = []restic.ID{node.Content[0]}
	isBinaryFile, err := checkBinaryFile(ctx, repo, tempNode)
	if err != nil || isBinaryFile {
		return isBinaryFile, nil, true, err
	}

	// 3. assemble a shorter file with a size just above the cutoff limit
	assembleShorterFile(repo, tempNode, node, diffSizeBytes)
	var fileBuf bytes.Buffer
	// 4. create "file"
	d := dump.New("", repo, &fileBuf)
	err = d.WriteNode(ctx, tempNode)
	return isBinaryFile, fileBuf.Bytes()[:diffSizeBytes], true, err
}

// isBinary: tests if the byte slice does not contain any 0x00 bytes
func isBinary(data []byte) bool {
	return bytes.IndexByte(data, 0) != -1
}

// DeepCopyJSON: taken from Gemini
func DeepCopyJSON(src interface{}, dst interface{}) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}
