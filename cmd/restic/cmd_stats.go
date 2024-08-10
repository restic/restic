package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/crypto"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/restorer"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/table"
	"github.com/restic/restic/internal/walker"

	"github.com/spf13/cobra"
)

var cmdStats = &cobra.Command{
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

Refer to the online manual for more details about each mode.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStats(cmd.Context(), statsOptions, globalOptions, args)
	},
}

// StatsOptions collects all options for the stats command.
type StatsOptions struct {
	// the mode of counting to perform (see consts for available modes)
	countMode string

	restic.SnapshotFilter
}

var statsOptions StatsOptions

func must(err error) {
	if err != nil {
		panic(fmt.Sprintf("error during setup: %v", err))
	}
}

func init() {
	cmdRoot.AddCommand(cmdStats)
	f := cmdStats.Flags()
	f.StringVar(&statsOptions.countMode, "mode", countModeRestoreSize, "counting mode: restore-size (default), files-by-contents, blobs-per-file or raw-data")
	must(cmdStats.RegisterFlagCompletionFunc("mode", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{countModeRestoreSize, countModeUniqueFilesByContents, countModeBlobsPerFile, countModeRawData}, cobra.ShellCompDirectiveDefault
	}))

	initMultiSnapshotFilter(f, &statsOptions.SnapshotFilter, true)
}

func runStats(ctx context.Context, opts StatsOptions, gopts GlobalOptions, args []string) error {
	err := verifyStatsInput(opts)
	if err != nil {
		return err
	}

	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock)
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

	if opts.countMode == countModeDebug {
		return statsDebug(ctx, repo)
	}

	if !gopts.JSON {
		Printf("scanning...\n")
	}

	// create a container for the stats (and other needed state)
	stats := &statsContainer{
		uniqueFiles:    make(map[fileID]struct{}),
		fileBlobs:      make(map[string]restic.IDSet),
		blobs:          restic.NewBlobSet(),
		SnapshotsCount: 0,
	}

	for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, &opts.SnapshotFilter, args) {
		err = statsWalkSnapshot(ctx, sn, repo, opts, stats)
		if err != nil {
			return fmt.Errorf("error walking snapshot: %v", err)
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if opts.countMode == countModeRawData {
		// the blob handles have been collected, but not yet counted
		for blobHandle := range stats.blobs {
			pbs := repo.LookupBlob(blobHandle.Type, blobHandle.ID)
			if len(pbs) == 0 {
				return fmt.Errorf("blob %v not found", blobHandle)
			}
			stats.TotalSize += uint64(pbs[0].Length)
			if repo.Config().Version >= 2 {
				stats.TotalUncompressedSize += uint64(crypto.CiphertextLength(int(pbs[0].DataLength())))
				if pbs[0].IsCompressed() {
					stats.TotalCompressedBlobsSize += uint64(pbs[0].Length)
					stats.TotalCompressedBlobsUncompressedSize += uint64(crypto.CiphertextLength(int(pbs[0].DataLength())))
				}
			}
			stats.TotalBlobCount++
		}
		if stats.TotalCompressedBlobsSize > 0 {
			stats.CompressionRatio = float64(stats.TotalCompressedBlobsUncompressedSize) / float64(stats.TotalCompressedBlobsSize)
		}
		if stats.TotalUncompressedSize > 0 {
			stats.CompressionProgress = float64(stats.TotalCompressedBlobsUncompressedSize) / float64(stats.TotalUncompressedSize) * 100
			stats.CompressionSpaceSaving = (1 - float64(stats.TotalSize)/float64(stats.TotalUncompressedSize)) * 100
		}
	}

	if gopts.JSON {
		err = json.NewEncoder(globalOptions.stdout).Encode(stats)
		if err != nil {
			return fmt.Errorf("encoding output: %v", err)
		}
		return nil
	}

	Printf("Stats in %s mode:\n", opts.countMode)
	Printf("     Snapshots processed:  %d\n", stats.SnapshotsCount)
	if stats.TotalBlobCount > 0 {
		Printf("        Total Blob Count:  %d\n", stats.TotalBlobCount)
	}
	if stats.TotalFileCount > 0 {
		Printf("        Total File Count:  %d\n", stats.TotalFileCount)
	}
	if stats.TotalUncompressedSize > 0 {
		Printf(" Total Uncompressed Size:  %-5s\n", ui.FormatBytes(stats.TotalUncompressedSize))
	}
	Printf("              Total Size:  %-5s\n", ui.FormatBytes(stats.TotalSize))
	if stats.CompressionProgress > 0 {
		Printf("    Compression Progress:  %.2f%%\n", stats.CompressionProgress)
	}
	if stats.CompressionRatio > 0 {
		Printf("       Compression Ratio:  %.2fx\n", stats.CompressionRatio)
	}
	if stats.CompressionSpaceSaving > 0 {
		Printf("Compression Space Saving:  %.2f%%\n", stats.CompressionSpaceSaving)
	}

	return nil
}

func statsWalkSnapshot(ctx context.Context, snapshot *restic.Snapshot, repo restic.Loader, opts StatsOptions, stats *statsContainer) error {
	if snapshot.Tree == nil {
		return fmt.Errorf("snapshot %s has nil tree", snapshot.ID().Str())
	}

	stats.SnapshotsCount++

	if opts.countMode == countModeRawData {
		// count just the sizes of unique blobs; we don't need to walk the tree
		// ourselves in this case, since a nifty function does it for us
		return restic.FindUsedBlobs(ctx, repo, restic.IDs{*snapshot.Tree}, stats.blobs, nil)
	}

	hardLinkIndex := restorer.NewHardlinkIndex[struct{}]()
	err := walker.Walk(ctx, repo, *snapshot.Tree, walker.WalkVisitor{
		ProcessNode: statsWalkTree(repo, opts, stats, hardLinkIndex),
	})
	if err != nil {
		return fmt.Errorf("walking tree %s: %v", *snapshot.Tree, err)
	}

	return nil
}

func statsWalkTree(repo restic.Loader, opts StatsOptions, stats *statsContainer, hardLinkIndex *restorer.HardlinkIndex[struct{}]) walker.WalkFunc {
	return func(parentTreeID restic.ID, npath string, node *restic.Node, nodeErr error) error {
		if nodeErr != nil {
			return nodeErr
		}
		if node == nil {
			return nil
		}

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
						if _, ok := stats.fileBlobs[nodePath]; !ok {
							stats.fileBlobs[nodePath] = restic.NewIDSet()
							stats.TotalFileCount++
						}
						if _, ok := stats.fileBlobs[nodePath][blobID]; !ok {
							// is always a data blob since we're accessing it via a file's Content array
							blobSize, found := repo.LookupBlobSize(restic.DataBlob, blobID)
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

			if node.Links == 1 || node.Type == "dir" {
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
func makeFileIDByContents(node *restic.Node) fileID {
	var bb []byte
	for _, c := range node.Content {
		bb = append(bb, []byte(c[:])...)
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
	blobs restic.BlobSet
}

// fileID is a 256-bit hash that distinguishes unique files.
type fileID [32]byte

const (
	countModeRestoreSize           = "restore-size"
	countModeUniqueFilesByContents = "files-by-contents"
	countModeBlobsPerFile          = "blobs-per-file"
	countModeRawData               = "raw-data"
	countModeDebug                 = "debug"
)

func statsDebug(ctx context.Context, repo restic.Repository) error {
	Warnf("Collecting size statistics\n\n")
	for _, t := range []restic.FileType{restic.KeyFile, restic.LockFile, restic.IndexFile, restic.PackFile} {
		hist, err := statsDebugFileType(ctx, repo, t)
		if err != nil {
			return err
		}
		Warnf("File Type: %v\n%v\n", t, hist)
	}

	hist, err := statsDebugBlobs(ctx, repo)
	if err != nil {
		return err
	}
	for _, t := range []restic.BlobType{restic.DataBlob, restic.TreeBlob} {
		Warnf("Blob Type: %v\n%v\n\n", t, hist[t])
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

	err := repo.ListBlobs(ctx, func(pb restic.PackedBlob) {
		hist[pb.Type].Add(uint64(pb.Length))
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

	out.WriteString(fmt.Sprintf("Count: %d\n", s.count))
	out.WriteString(fmt.Sprintf("Total Size: %s\n", ui.FormatBytes(s.totalSize)))

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
		out.WriteString(fmt.Sprintf("Oversized: %v\n", s.oversized))
	}
	return out.String()
}
