package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
	statsui "github.com/restic/restic/internal/ui/stats"
	"github.com/restic/restic/internal/ui/table"
	"github.com/restic/restic/internal/walker"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newStatsCommand(globalOptions *global.Options) *cobra.Command {
	var opts StatsOptions

	cmd := &cobra.Command{
		Use:   "stats [flags] [snapshot ID] [...]",
		Short: "Scan the repository and show basic statistics",
		Long: `
The "stats" command walks one or multiple snapshots in a repository
and accumulates statistics about the data stored therein. It reports
on the number of unique files and their sizes, according to one of
the counting modes as given by the --mode flag.

It operates on all snapshots matching the selection criteria or all
snapshots if nothing is specified. The special snapshot ID "latest"
is also supported. Some modes make more sense over
just a single snapshot, while others are useful across all snapshots,
depending on what you are trying to calculate.

The modes are:

* restore-size: (default) Counts the size of the restored files.
* files-by-contents: Counts total size of unique files, where a file is
   considered unique if it has unique contents.
* raw-data: Counts the size of blobs in the repository, regardless of
  how many files reference them.
* blobs-per-file: A combination of files-by-contents and raw-data.
* info: Repository-wide overview combining all easily-accessible statistics.
  Reports unique files, used blobs, unused blobs, packfile status,
  duplicate index entries and total/used/unused sizes.

Refer to the online manual for more details about each mode.

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
			return runStats(cmd.Context(), opts, *globalOptions, args, globalOptions.Term)
		},
	}

	opts.AddFlags(cmd.Flags())
	must(cmd.RegisterFlagCompletionFunc("mode", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{countModeRestoreSize, countModeUniqueFilesByContents, countModeBlobsPerFile, countModeRawData, countModeInfo}, cobra.ShellCompDirectiveDefault
	}))
	return cmd
}

// StatsOptions collects all options for the stats command.
type StatsOptions struct {
	// the mode of counting to perform (see consts for available modes)
	countMode string

	data.SnapshotFilter
}

func (opts *StatsOptions) AddFlags(f *pflag.FlagSet) {
	f.StringVar(&opts.countMode, "mode", countModeRestoreSize, "counting mode: restore-size (default), files-by-contents, blobs-per-file, raw-data or info")
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)
}

func must(err error) {
	if err != nil {
		panic(fmt.Sprintf("error during setup: %v", err))
	}
}

func runStats(ctx context.Context, opts StatsOptions, gopts global.Options, args []string, term ui.Terminal) error {
	err := verifyStatsInput(opts)
	if err != nil {
		return err
	}

	printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, term)

	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock, printer)
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

	if opts.countMode == countModeDebug {
		return statsDebug(ctx, repo, printer)
	}

	// create a container for the stats (and other needed state)
	stats := &statsContainer{
		uniqueFiles:    make(map[fileID]struct{}),
		fileBlobs:      make(map[string]restic.IDSet),
		blobs:          repo.NewAssociatedBlobSet(),
		SnapshotsCount: 0,
	}

	var snapshots data.Snapshots
	// info mode: collect all snapshot roots, then do one data.StreamTrees
	if opts.countMode == countModeInfo {
		var roots restic.IDs
		for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, &opts.SnapshotFilter, args, printer) {
			roots = append(roots, *sn.Tree)
			stats.SnapshotsCount++
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		out := &infoStats{
			uniqueFiles: make(map[fileID]uint64),
		}
		out.General.SnapshotsCount = stats.SnapshotsCount

		if err := out.statsInfoStreamTrees(ctx, repo, roots, stats); err != nil {
			return err
		}
		return out.runStatsInfo(ctx, repo, stats, gopts, printer)
	}

	for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, &opts.SnapshotFilter, args, printer) {
		snapshots = append(snapshots, sn)
	}

	statsProgress := statsui.NewProgress(term, gopts.Quiet, gopts.JSON, uint64(len(snapshots)))
	defer statsProgress.Done()

	for _, sn := range snapshots {
		err = statsWalkSnapshot(ctx, sn, repo, opts, stats, statsProgress)
		if err != nil {
			return fmt.Errorf("error walking snapshot: %v", err)
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if opts.countMode == countModeRawData {
		// the blob handles have been collected, but not yet counted
		for blobHandle := range stats.blobs.Keys() {
			pbs := repo.LookupBlob(blobHandle)
			if len(pbs) == 0 {
				return fmt.Errorf("blob %v not found", blobHandle)
			}
			stats.TotalSize += uint64(pbs[0].CiphertextLength())
			if repo.Config().Version >= 2 {
				stats.TotalUncompressedSize += uint64(pbs[0].UncompressedCiphertextLength())
				if pbs[0].IsCompressed() {
					stats.TotalCompressedBlobsSize += uint64(pbs[0].CiphertextLength())
					stats.TotalCompressedBlobsUncompressedSize += uint64(pbs[0].UncompressedCiphertextLength())
				}
			}
			stats.TotalBlobCount++
			statsProgress.Update(0, 1, uint64(pbs[0].CiphertextLength()))
		}
		if stats.TotalCompressedBlobsSize > 0 {
			stats.CompressionRatio = float64(stats.TotalCompressedBlobsUncompressedSize) / float64(stats.TotalCompressedBlobsSize)
		}
		if stats.TotalUncompressedSize > 0 {
			stats.CompressionProgress = float64(stats.TotalCompressedBlobsUncompressedSize) / float64(stats.TotalUncompressedSize) * 100
			stats.CompressionSpaceSaving = (1 - float64(stats.TotalSize)/float64(stats.TotalUncompressedSize)) * 100
		}
	}
	// stop progress bar to prevent mangled output
	statsProgress.Done()

	if gopts.JSON {
		err = json.NewEncoder(gopts.Term.OutputWriter()).Encode(stats)
		if err != nil {
			return fmt.Errorf("encoding output: %v", err)
		}
		return nil
	}

	printer.S("Stats in %s mode:", opts.countMode)
	printer.S("     Snapshots processed:  %d", stats.SnapshotsCount)
	if stats.TotalBlobCount > 0 {
		printer.S("        Total Blob Count:  %d", stats.TotalBlobCount)
	}
	if stats.TotalFileCount > 0 {
		printer.S("        Total File Count:  %d", stats.TotalFileCount)
	}
	if stats.TotalUncompressedSize > 0 {
		printer.S(" Total Uncompressed Size:  %-5s", ui.FormatBytes(stats.TotalUncompressedSize))
	}
	printer.S("              Total Size:  %-5s", ui.FormatBytes(stats.TotalSize))
	if stats.CompressionProgress > 0 {
		printer.S("    Compression Progress:  %.2f%%", stats.CompressionProgress)
	}
	if stats.CompressionRatio > 0 {
		printer.S("       Compression Ratio:  %.2fx", stats.CompressionRatio)
	}
	if stats.CompressionSpaceSaving > 0 {
		printer.S("Compression Space Saving:  %.2f%%", stats.CompressionSpaceSaving)
	}

	return nil
}

func statsWalkSnapshot(ctx context.Context, snapshot *data.Snapshot, repo restic.Loader, opts StatsOptions, stats *statsContainer, sp *statsui.Progress) error {
	sp.ProcessSnapshot()
	if snapshot.Tree == nil {
		return fmt.Errorf("snapshot %s has nil tree", snapshot.ID().Str())
	}

	stats.SnapshotsCount++

	if opts.countMode == countModeRawData {
		// count just the sizes of unique blobs; we don't need to walk the tree
		// ourselves in this case, since a nifty function does it for us
		return data.FindUsedBlobs(ctx, repo, restic.IDs{*snapshot.Tree}, stats.blobs, restic.NoopCounter)
	}

	hardLinkIndex := data.NewHardlinkIndex[struct{}]()
	err := walker.Walk(ctx, repo, *snapshot.Tree, walker.WalkVisitor{
		ProcessNode: statsWalkTree(repo, opts, stats, hardLinkIndex, sp),
	})
	if err != nil {
		return fmt.Errorf("walking tree %s: %v", *snapshot.Tree, err)
	}

	return nil
}

func statsWalkTree(repo restic.Loader, opts StatsOptions, stats *statsContainer, hardLinkIndex *data.HardlinkIndex[struct{}], progress *statsui.Progress) walker.WalkFunc {
	return func(parentTreeID restic.ID, npath string, node *data.Node, nodeErr error) error {
		if nodeErr != nil {
			return nodeErr
		}
		if node == nil {
			return nil
		}
		progress.Update(1, 0, uint64(node.Size))
		if opts.countMode == countModeUniqueFilesByContents || opts.countMode == countModeBlobsPerFile {
			// only count this file if we haven't visited it before
			fid := makeFileIDByContents(node)
			if _, ok := stats.uniqueFiles[fid]; !ok {
				// mark the file as visited
				stats.uniqueFiles[fid] = struct{}{}

				if opts.countMode == countModeUniqueFilesByContents {
					// simply count the size of each unique file (unique by contents only)
					stats.TotalSize += node.Size
					stats.TotalFileCount++
				}
				if opts.countMode == countModeBlobsPerFile {
					// count the size of each unique blob reference, which is
					// by unique file (unique by contents and file path)
					for _, blobID := range node.Content {
						// ensure we have this file (by path) in our map; in this
						// mode, a file is unique by both contents and path
						nodePath := filepath.Join(npath, node.Name)
						progress.Update(0, 1, 0)
						if _, ok := stats.fileBlobs[nodePath]; !ok {
							stats.fileBlobs[nodePath] = restic.NewIDSet()
							stats.TotalFileCount++
						}
						if _, ok := stats.fileBlobs[nodePath][blobID]; !ok {
							// is always a data blob since we're accessing it via a file's Content array
							blobSize, found := repo.LookupBlobSize(restic.BlobHandle{Type: restic.DataBlob, ID: blobID})
							if !found {
								return fmt.Errorf("blob %s not found for tree %s", blobID, parentTreeID)
							}

							// count the blob's size, then add this blob by this
							// file (path) so we don't double-count it
							stats.TotalSize += uint64(blobSize)
							stats.fileBlobs[nodePath].Insert(blobID)
							// this mode also counts total unique blob _references_ per file
							stats.TotalBlobCount++
						}
					}
				}
			}
		}

		if opts.countMode == countModeRestoreSize {
			// as this is a file in the snapshot, we can simply count its
			// size without worrying about uniqueness, since duplicate files
			// will still be restored
			stats.TotalFileCount++

			if node.Links == 1 || node.Type == data.NodeTypeDir {
				stats.TotalSize += node.Size
			} else {
				// if hardlinks are present only count each deviceID+inode once
				if !hardLinkIndex.Has(node.Inode, node.DeviceID) || node.Inode == 0 {
					hardLinkIndex.Add(node.Inode, node.DeviceID, struct{}{})
					stats.TotalSize += node.Size
				}
			}
		}
		return nil
	}
}

// makeFileIDByContents returns a hash of the blob IDs of the
// node's Content in sequence.
func makeFileIDByContents(node *data.Node) fileID {
	var bb []byte
	for _, c := range node.Content {
		bb = append(bb, c[:]...)
	}
	return sha256.Sum256(bb)
}

func verifyStatsInput(opts StatsOptions) error {
	// require a recognized counting mode
	switch opts.countMode {
	case countModeRestoreSize:
	case countModeUniqueFilesByContents:
	case countModeBlobsPerFile:
	case countModeRawData:
	case countModeInfo:
	case countModeDebug:
	default:
		return fmt.Errorf("unknown counting mode: %s (use the -h flag to get a list of supported modes)", opts.countMode)
	}

	return nil
}

// statsContainer holds information during a walk of a repository
// to collect information about it, as well as state needed
// for a successful and efficient walk.
type statsContainer struct {
	TotalSize                            uint64  `json:"total_size"`
	TotalUncompressedSize                uint64  `json:"total_uncompressed_size,omitempty"`
	TotalCompressedBlobsSize             uint64  `json:"-"`
	TotalCompressedBlobsUncompressedSize uint64  `json:"-"`
	CompressionRatio                     float64 `json:"compression_ratio,omitempty"`
	CompressionProgress                  float64 `json:"compression_progress,omitempty"`
	CompressionSpaceSaving               float64 `json:"compression_space_saving,omitempty"`
	TotalFileCount                       uint64  `json:"total_file_count,omitempty"`
	TotalBlobCount                       uint64  `json:"total_blob_count,omitempty"`
	// holds count of all considered snapshots
	SnapshotsCount int `json:"snapshots_count"`

	// uniqueFiles marks visited files according to their
	// contents (hashed sequence of content blob IDs)
	uniqueFiles map[fileID]struct{}

	// fileBlobs maps a file name (path) to the set of
	// blobs that have been seen as a part of the file
	fileBlobs map[string]restic.IDSet

	// blobs is used to count individual unique blobs,
	// independent of references to files
	blobs restic.AssociatedBlobSet
}

// fileID is a 256-bit hash that distinguishes unique files.
type fileID [32]byte

const (
	countModeRestoreSize           = "restore-size"
	countModeUniqueFilesByContents = "files-by-contents"
	countModeBlobsPerFile          = "blobs-per-file"
	countModeRawData               = "raw-data"
	countModeInfo                  = "info"
	countModeDebug                 = "debug"
)

func statsDebug(ctx context.Context, repo restic.Repository, printer restic.Printer) error {
// infoStats is the output structure for --mode info.
type infoStats struct {
	General struct {
		SnapshotsCount  int    `json:"snapshots"`
		SizeSnapshots   uint64 `json:"size_snapshots,omitempty"`
		TreeCount       int    `json:"tree_roots"`
		CountIndexFiles int    `json:"index_files,omitempty"`
		SizeIndexFiles  uint64 `json:"size_index_files,omitempty"`
	} `json:"general"`

	// counts and sizes from unique (by content) files
	UniqueFiles struct {
		UniqueFilesByContents int    `json:"unique_files_by_contents"`
		SizeUniqueFiles       uint64 `json:"size_unique_files"`
	} `json:"unique_files"`

	// Blob statistics from the index
	Blobs struct {
		TotalIndexedBlobs int    `json:"total_indexed_blobs"` // all entries in the index
		TotalSize         uint64 `json:"total_size"`
		UsedBlobs         int    `json:"used_blobs"` // referenced by snapshots
		UsedSize          uint64 `json:"used_size"`
		UnusedBlobs       int    `json:"unused_blobs,omitempty"` // in index but unreferenced
		UnusedSize        uint64 `json:"unused_size,omitempty"`
		DuplicateBlobRefs int    `json:"duplicate_blobs,omitempty"`
		SizeDuplicates    uint64 `json:"size_duplicates,omitempty"`
		TreeBlobs         int    `json:"tree_blobs"`
		SizeTreeBlobs     uint64 `json:"tree_size"`
		UcSizeTreeBlobs   uint64 `json:"uncompressed_size_tree_blobs,omitempty"`
		DataBlobs         int    `json:"data_blobs"`
		SizeDataBlobs     uint64 `json:"data_size"`
		UcSizeDataBlobs   uint64 `json:"uncompressed_size_data_blobs,omitempty"`
	} `json:"blobs"`

	// nodes and trees
	Trees struct {
		CountTrees       int `json:"trees"`              // all these counts are approximate
		CountNodes       int `json:"nodes"`              // since it is most likely that all
		CountAllFiles    int `json:"files"`              // trees are not visited in the same
		CountAllDirs     int `json:"directories"`        // same order when data.StreamTrees is called
		CountAllSymlinks int `json:"symlinks,omitempty"` // shared trees are only visited once
		CountAllOthers   int `json:"node_other,omitempty"`
	} `json:"trees"`

	Packfiles struct {
		TotalPackFiles        int    `json:"total_packfiles"`
		CountTreePackfiles    int    `json:"tree_packfiles"`
		SizeTreePackfiles     uint64 `json:"size_tree_packfiles"`
		CountDataPackfiles    int    `json:"data_packfiles"`
		SizeDataPackfiles     uint64 `json:"size_data_packfiles"`
		CountFullPackfiles    int    `json:"full_packfiles"`
		SizeFullPackfiles     uint64 `json:"size_full_packfiles"`
		CountPartialPackfiles int    `json:"partial_packfiles,omitempty"`
		SizeFullPartial       uint64 `json:"size_partial_packfiles,omitempty"` // size of partial packfile, all blobs
	} `json:"packfiles"`

	// compression
	Compression struct {
		TotalUncompressedSize  uint64  `json:"total_uncompressed_size,omitempty"`
		UsedUncompressedSize   uint64  `json:"used_uncompressed_size,omitempty"`
		CompressionRatio       float64 `json:"compression_ratio,omitempty"`
		CompressionProgress    float64 `json:"compression_progress,omitempty"`
		CompressionSpaceSaving float64 `json:"compression_space_saving,omitempty"`
	} `json:"compression"`

	compressedStoredSize       uint64
	compressedUncompressedSize uint64

	// storage items
	uniqueFiles    map[fileID]uint64
	packsFromIndex map[restic.ID]int64

	// fully unused packfiles
	FullyUnused struct {
		FullyUnusedCount      int    `json:"unused_packfiles,omitempty"`
		FullyUnusedBlobsCount int    `json:"unused_packfiles_blobs,omitempty"`
		FullyUnusedPackSize   uint64 `json:"unused_packfiles_size,omitempty"`
		FullyUnusedBlobsSize  uint64 `json:"unused_packfiles_blobs_size,omitempty"`
	} `json:"fully_unused"`
}

// processTrees processes one tree and counts various node types
func (out *infoStats) processTrees(_ restic.ID, nodes data.TreeNodeIterator,
	stats *statsContainer, lock *sync.Mutex,
) error {
	for item := range nodes {
		if item.Error != nil {
			return item.Error
		}
		node := item.Node

		lock.Lock()
		out.Trees.CountNodes++

		switch node.Type {
		case data.NodeTypeFile:
			out.Trees.CountAllFiles++
			for _, blobID := range node.Content {
				stats.blobs.Insert(restic.BlobHandle{ID: blobID, Type: restic.DataBlob})
			}
			out.uniqueFiles[makeFileIDByContents(node)] = node.Size

		case data.NodeTypeDir:
			out.Trees.CountAllDirs++

		case data.NodeTypeSymlink:
			out.Trees.CountAllSymlinks++

		default:
			out.Trees.CountAllOthers++
		}
		lock.Unlock()
	}

	return nil
}

// statsInfoStreamTrees uses data.StreamTrees to walk all roots in a single parallel pass.
func (out *infoStats) statsInfoStreamTrees(ctx context.Context, repo restic.Loader,
	roots restic.IDs, stats *statsContainer,
) error {
	var lock sync.Mutex
	out.General.TreeCount = len(roots)
	err := data.StreamTrees(ctx, repo, roots, nil,
		func(treeID restic.ID) bool {
			h := restic.BlobHandle{ID: treeID, Type: restic.TreeBlob}
			lock.Lock()
			visited := stats.blobs.Has(h)
			stats.blobs.Insert(h)
			out.Trees.CountTrees++
			lock.Unlock()
			return visited
		},
		func(id restic.ID, err error, nodes data.TreeNodeIterator) error {
			if err != nil {
				return err
			}
			return out.processTrees(id, nodes, stats, &lock)
		},
	)

	if err != nil {
		return err
	}

	out.UniqueFiles.UniqueFilesByContents = len(out.uniqueFiles)
	for _, size := range out.uniqueFiles {
		out.UniqueFiles.SizeUniqueFiles += size
	}

	return nil
}

// printStats prints the result of --mode info in text mode
func (out *infoStats) printStats(printer progress.Printer) {
	printer.S("Stats in info mode:")

	printer.S("")
	printer.S("%-28s %8s  %12s %12s", "Type", "Count", "Compressed", "Uncompressed")
	printer.S("%-28s %8d  %12s %12s", "indexed tree blobs",
		out.Blobs.TreeBlobs, ui.FormatBytes(out.Blobs.SizeTreeBlobs),
		ui.FormatBytes(out.Blobs.UcSizeTreeBlobs))
	printer.S("%-28s %8d  %12s %12s", "indexed data blobs",
		out.Blobs.DataBlobs, ui.FormatBytes(out.Blobs.SizeDataBlobs),
		ui.FormatBytes(out.Blobs.UcSizeDataBlobs))
	printer.S("%-28s %8d  %12s %12s", "indexed all  blobs",
		out.Blobs.TreeBlobs+out.Blobs.DataBlobs,
		ui.FormatBytes(out.Blobs.SizeTreeBlobs+out.Blobs.SizeDataBlobs),
		ui.FormatBytes(out.Blobs.UcSizeTreeBlobs+out.Blobs.UcSizeDataBlobs))
	if out.General.SizeSnapshots > 0 {
		printer.S("%-28s %8d  %12s", "Snapshots processed",
			out.General.SnapshotsCount, ui.FormatBytes(out.General.SizeSnapshots))
	} else {
		printer.S("%-28s %8d", "Snapshots processed", out.General.SnapshotsCount)
	}
	printer.S("%-28s %8d", "Trees processed", out.General.TreeCount)
	if out.General.CountIndexFiles > 0 {
		printer.S("%-28s %8d  %12s", "Index files",
			out.General.CountIndexFiles, ui.FormatBytes(out.General.SizeIndexFiles))
	}
	printer.S("")
	printer.S("Blobs (from index)")
	printer.S("%-28s %8d  %12s", "Used blobs",
		out.Blobs.UsedBlobs, ui.FormatBytes(out.Blobs.UsedSize))
	if out.Blobs.UnusedBlobs > 0 {
		printer.S("%-28s %8d  %12s", "Unused blobs", out.Blobs.UnusedBlobs,
			ui.FormatBytes(out.Blobs.UnusedSize))
	}
	if out.FullyUnused.FullyUnusedBlobsCount > 0 {
		printer.S("%-28s %8d  %12s", "unreferenced blobs",
			out.FullyUnused.FullyUnusedBlobsCount, ui.FormatBytes(out.FullyUnused.FullyUnusedBlobsSize))
	}
	if out.Blobs.DuplicateBlobRefs > 0 {
		printer.S("%-28s %8d  %12s", "Unused duplicate",
			out.Blobs.DuplicateBlobRefs, ui.FormatBytes(out.Blobs.SizeDuplicates))
	}
	if out.Blobs.UnusedBlobs+out.FullyUnused.FullyUnusedBlobsCount+out.Blobs.DuplicateBlobRefs > 0 {
		unusedSize := out.Blobs.UnusedSize + out.FullyUnused.FullyUnusedBlobsSize + out.Blobs.SizeDuplicates
		unusedRatio := 100 * float64(unusedSize) / float64(out.Blobs.SizeTreeBlobs+out.Blobs.SizeDataBlobs)
		printer.S("%-28s %7.1f%%", "unused ratio", unusedRatio)
	}

	printer.S("")
	printer.S("%-28s %8d", "all trees", out.Trees.CountTrees)
	printer.S("%-28s %8d", "all tree nodes", out.Trees.CountNodes)
	printer.S("%-28s %8d", "all files", out.Trees.CountAllFiles)
	printer.S("%-28s %8d", "all directories", out.Trees.CountAllDirs)
	if out.Trees.CountAllSymlinks > 0 {
		printer.S("%-28s %8d", "all symlinks", out.Trees.CountAllSymlinks)
	}
	if out.Trees.CountAllOthers > 0 {
		printer.S("%-28s %8d", "all other node types", out.Trees.CountAllOthers)
	}

	printer.S("")
	printer.S("Files")
	printer.S("%-28s %8d  %12s %12s",
		"Unique (by contents)",
		out.UniqueFiles.UniqueFilesByContents, "", ui.FormatBytes(out.UniqueFiles.SizeUniqueFiles))

	printer.S("")
	printer.S("Packfiles")
	printer.S("%-28s %8d  %12s", "tree packfiles",
		out.Packfiles.CountTreePackfiles, ui.FormatBytes(out.Packfiles.SizeTreePackfiles))
	printer.S("%-28s %8d  %12s", "data packfiles",
		out.Packfiles.CountDataPackfiles, ui.FormatBytes(out.Packfiles.SizeDataPackfiles))
	if out.FullyUnused.FullyUnusedPackSize > 0 {
		printer.S("%-28s %8d  %12s", "unreferenced packfiles",
			out.FullyUnused.FullyUnusedCount, ui.FormatBytes(out.FullyUnused.FullyUnusedPackSize))
	}
	if out.Packfiles.CountPartialPackfiles > 0 {
		printer.S("%-28s %8d  %12s", "partially used packfiles",
			out.Packfiles.CountPartialPackfiles, ui.FormatBytes(out.Packfiles.SizeFullPartial))
	}
	printer.S("%-28s %8d  %12s", "fully used packfiles",
		out.Packfiles.CountFullPackfiles, ui.FormatBytes(out.Packfiles.SizeFullPackfiles))

	printer.S("%-28s %8d  %12s", "all packfiles",
		out.Packfiles.TotalPackFiles, ui.FormatBytes(out.Packfiles.SizeTreePackfiles+out.Packfiles.SizeDataPackfiles))

	if out.Compression.TotalUncompressedSize > 0 {
		printer.S("")
		printer.S("Compression (repository v2)")
		printer.S("%-28s %8s  %12s %12s", "Total uncompressed", "", "",
			ui.FormatBytes(out.Compression.TotalUncompressedSize))
		printer.S("%-28s %8s  %12s %12s", "Used uncompressed", "", "",
			ui.FormatBytes(out.Compression.UsedUncompressedSize))
		if out.Compression.CompressionProgress > 0 {
			printer.S("%-28s %7.1f%%", "Compression progress", out.Compression.CompressionProgress)
			if out.Compression.CompressionRatio >= 1 {
				printer.S("%-28s %7.1fx", "Compression ratio", out.Compression.CompressionRatio)
			}
			printer.S("%-28s %7.1f%%", "Compression space saved", out.Compression.CompressionSpaceSaving)
		}
	}
}

// copied from intermal/repository/prune.go
type packInfoStats struct {
	usedBlobs      int
	unusedBlobs    int
	duplicateBlobs int
	usedSize       uint64
	unusedSize     uint64
	tpe            restic.BlobType
}

// processIndexRecords walks the Master Index and separates blobs into
// used / unused / duplicate
// countBlobsAndSizes walks the Master Index count tree and data blobs/sizes
// also make note of the encompassing packfile, counting duplicates as well
func (out *infoStats) processIndexRecords(ctx context.Context, repo restic.Repository,
	stats *statsContainer,
) error {
	seenHandles := repo.NewAssociatedBlobSet()
	indexPack := make(map[restic.ID]packInfoStats)
	treePackfiles := restic.NewIDSet()
	dataPackfiles := restic.NewIDSet()
	err := repo.ListBlobs(ctx, func(pb restic.PackedBlob) {
		out.Blobs.TotalIndexedBlobs++
		stored := uint64(pb.Length)
		out.Blobs.TotalSize += stored

		switch pb.Type {
		case restic.TreeBlob:
			out.Blobs.TreeBlobs++
			out.Blobs.SizeTreeBlobs += uint64(pb.Length)
			out.Blobs.UcSizeTreeBlobs += uint64(pb.UncompressedLength)
			treePackfiles.Insert(pb.PackID)
		case restic.DataBlob:
			out.Blobs.DataBlobs++
			out.Blobs.SizeDataBlobs += uint64(pb.Length)
			out.Blobs.UcSizeDataBlobs += uint64(pb.UncompressedLength)
			dataPackfiles.Insert(pb.PackID)
		}

		ip := indexPack[pb.PackID] // new empty packInfoStats entry
		if ip.tpe == restic.InvalidBlob {
			ip.tpe = pb.Type
		}

		var uncompLen uint64
		if repo.Config().Version >= 2 {
			uncompLen = uint64(crypto.CiphertextLength(int(pb.DataLength())))
			out.Compression.TotalUncompressedSize += uncompLen
			if pb.IsCompressed() {
				out.compressedStoredSize += stored
				out.compressedUncompressedSize += uncompLen
			}
		}

		handle := restic.BlobHandle{ID: pb.ID, Type: pb.Type}
		alreadyThere := seenHandles.Has(handle)
		if alreadyThere {
			out.Blobs.DuplicateBlobRefs++
			out.Blobs.SizeDuplicates += uint64(pb.Length)
			ip.duplicateBlobs++
			indexPack[pb.PackID] = ip
			return
		}

		if stats.blobs.Has(handle) {
			out.Blobs.UsedBlobs++
			out.Blobs.UsedSize += stored
			ip.usedBlobs++
			ip.usedSize += stored
		} else {
			out.Blobs.UnusedBlobs++
			out.Blobs.UnusedSize += stored
			ip.unusedBlobs++
			ip.unusedSize += stored
		}
		seenHandles.Insert(handle)

		if repo.Config().Version >= 2 {
			out.Compression.UsedUncompressedSize += uncompLen
		}
		// update stats
		indexPack[pb.PackID] = ip
	})
	if err != nil {
		return err
	}

	// classify packfile counts and sizes
	for packID, ip := range indexPack {
		packSize := uint64(out.packsFromIndex[packID])
		if ip.unusedBlobs == 0 && ip.duplicateBlobs == 0 {
			out.Packfiles.CountFullPackfiles++
			out.Packfiles.SizeFullPackfiles += packSize
		} else if ip.usedBlobs > 0 {
			out.Packfiles.CountPartialPackfiles++
			out.Packfiles.SizeFullPartial += packSize
		} else {
			out.FullyUnused.FullyUnusedCount++
			out.FullyUnused.FullyUnusedPackSize += packSize
		}
	}

	// size the tree and data packfiles
	for packID := range treePackfiles {
		out.Packfiles.SizeTreePackfiles += uint64(out.packsFromIndex[packID])
	}
	for packID := range dataPackfiles {
		out.Packfiles.SizeDataPackfiles += uint64(out.packsFromIndex[packID])
	}
	out.Packfiles.CountTreePackfiles = len(treePackfiles)
	out.Packfiles.CountDataPackfiles = len(dataPackfiles)
	out.Packfiles.TotalPackFiles = len(treePackfiles) + len(dataPackfiles)

	return nil
}

// runStatsInfo enumerates the Master Index to classify
// every blob as used or unused, counts packfiles, accumulates sizes, and
// prints the results.
func (out *infoStats) runStatsInfo(ctx context.Context, repo restic.Repository,
	stats *statsContainer, gopts global.Options,
	printer progress.Printer,
) error {

	var err error
	// size and count physical files: snapshots and index
	// the test functions act here and forbid a second reading of the index files
	// and snapshot files.
	/*for i, tpe := range []restic.FileType{restic.IndexFile, restic.SnapshotFile} {
		err = repo.List(ctx, tpe, func(_ restic.ID, size int64) error {
			switch i {
			case 0: // index
				out.General.CountIndexFiles++
				out.General.SizeIndexFiles += uint64(size)
			case 1: // snapshots
				out.General.SizeSnapshots += uint64(size)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}*/

	out.packsFromIndex, err = pack.Size(ctx, repo, false)
	if err != nil {
		return err
	}

	if err = out.processIndexRecords(ctx, repo, stats); err != nil {
		return err
	}

	if out.compressedStoredSize > 0 {
		out.Compression.CompressionRatio = math.Round(100*float64(out.compressedUncompressedSize)/
			float64(out.compressedStoredSize)) / 100
	}
	if out.Compression.TotalUncompressedSize > 0 {
		out.Compression.CompressionProgress = math.Round(1000*float64(out.compressedUncompressedSize)/
			float64(out.Compression.TotalUncompressedSize)) / 10
		out.Compression.CompressionSpaceSaving = math.Round(1000-float64(1000*out.Blobs.TotalSize)/
			float64(out.Compression.TotalUncompressedSize)) / 10
	}

	if gopts.JSON {
		return json.NewEncoder(gopts.Term.OutputWriter()).Encode(out)
	}
	out.printStats(printer)

	return nil
}

func statsDebug(ctx context.Context, repo restic.Repository, printer progress.Printer) error {
	printer.E("Collecting size statistics\n\n")
	for _, t := range []restic.FileType{restic.KeyFile, restic.LockFile, restic.IndexFile, restic.SnapshotFile, restic.PackFile} {
		hist, err := statsDebugFileType(ctx, repo, t)
		if err != nil {
			return err
		}
		printer.E("File Type: %v\n%v", t, hist)
	}

	hist, err := statsDebugBlobs(ctx, repo)
	if err != nil {
		return err
	}
	for _, t := range []restic.BlobType{restic.DataBlob, restic.TreeBlob} {
		printer.E("Blob Type: %v\n%v\n\n", t, hist[t])
	}

	return nil
}

func statsDebugFileType(ctx context.Context, repo restic.Lister, tpe restic.FileType) (*sizeHistogram, error) {
	hist := newSizeHistogram(2 * repository.MaxPackSize)
	err := repo.List(ctx, tpe, func(_ restic.ID, size int64) error {
		hist.Add(uint64(size))
		return nil
	})

	return hist, err
}

func statsDebugBlobs(ctx context.Context, repo restic.Repository) ([restic.NumBlobTypes]*sizeHistogram, error) {
	var hist [restic.NumBlobTypes]*sizeHistogram
	for i := 0; i < len(hist); i++ {
		hist[i] = newSizeHistogram(2 * chunker.MaxSize)
	}

	err := repo.ListBlobs(ctx, func(pb restic.PackBlob) {
		hist[pb.Handle().Type].Add(uint64(pb.CiphertextLength()))
	})

	return hist, err
}

type sizeClass struct {
	lower, upper uint64
	count        int64
}

type sizeHistogram struct {
	count     int64
	totalSize uint64
	buckets   []sizeClass
	oversized []uint64
}

func newSizeHistogram(sizeLimit uint64) *sizeHistogram {
	h := &sizeHistogram{}
	h.buckets = append(h.buckets, sizeClass{0, 0, 0})

	lowerBound := uint64(1)
	growthFactor := uint64(10)

	for lowerBound < sizeLimit {
		upperBound := lowerBound*growthFactor - 1
		if upperBound > sizeLimit {
			upperBound = sizeLimit
		}
		h.buckets = append(h.buckets, sizeClass{lowerBound, upperBound, 0})
		lowerBound *= growthFactor
	}

	return h
}

func (s *sizeHistogram) Add(size uint64) {
	s.count++
	s.totalSize += size

	for i, bucket := range s.buckets {
		if size >= bucket.lower && size <= bucket.upper {
			s.buckets[i].count++
			return
		}
	}

	s.oversized = append(s.oversized, size)
}

func (s sizeHistogram) String() string {
	var out strings.Builder

	fmt.Fprintf(&out, "Count: %d\n", s.count)
	fmt.Fprintf(&out, "Total Size: %s\n", ui.FormatBytes(s.totalSize))

	t := table.New()
	t.AddColumn("Size", "{{.SizeRange}}")
	t.AddColumn("Count", "{{.Count}}")
	type line struct {
		SizeRange string
		Count     int64
	}

	// only print up to the highest used bucket size
	lastFilledIdx := 0
	for i := 0; i < len(s.buckets); i++ {
		if s.buckets[i].count != 0 {
			lastFilledIdx = i
		}
	}

	var lines []line
	hasStarted := false
	for i, b := range s.buckets {
		if i > lastFilledIdx {
			break
		}

		if b.count > 0 {
			hasStarted = true
		}
		if hasStarted {
			lines = append(lines, line{
				SizeRange: fmt.Sprintf("%d - %d Byte", b.lower, b.upper),
				Count:     b.count,
			})
		}
	}
	longestRange := 0
	for _, l := range lines {
		if longestRange < len(l.SizeRange) {
			longestRange = len(l.SizeRange)
		}
	}
	for i := range lines {
		lines[i].SizeRange = strings.Repeat(" ", longestRange-len(lines[i].SizeRange)) + lines[i].SizeRange
		t.AddRow(lines[i])
	}

	_ = t.Write(&out)

	if len(s.oversized) > 0 {
		fmt.Fprintf(&out, "Oversized: %v\n", s.oversized)
	}
	return out.String()
}
