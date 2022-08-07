package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"

	"github.com/minio/sha256-simd"
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
* files-by-contents: Counts total size of files, where a file is
   considered unique if it has unique contents.
* raw-data: Counts the size of blobs in the repository, regardless of
  how many files reference them.
* blobs-per-file: A combination of files-by-contents and raw-data.

Refer to the online manual for more details about each mode.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStats(globalOptions, args)
	},
}

// StatsOptions collects all options for the stats command.
type StatsOptions struct {
	// the mode of counting to perform (see consts for available modes)
	countMode string

	// filter snapshots by, if given by user
	Hosts []string
	Tags  restic.TagLists
	Paths []string
}

var statsOptions StatsOptions

func init() {
	cmdRoot.AddCommand(cmdStats)
	f := cmdStats.Flags()
	f.StringVar(&statsOptions.countMode, "mode", countModeRestoreSize, "counting mode: restore-size (default), files-by-contents, blobs-per-file or raw-data")
	f.StringArrayVarP(&statsOptions.Hosts, "host", "H", nil, "only consider snapshots with the given `host` (can be specified multiple times)")
	f.Var(&statsOptions.Tags, "tag", "only consider snapshots which include this `taglist` in the format `tag[,tag,...]` (can be specified multiple times)")
	f.StringArrayVar(&statsOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path` (can be specified multiple times)")
}

func runStats(gopts GlobalOptions, args []string) error {
	err := verifyStatsInput(gopts, args)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		lock, err := lockRepo(ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	snapshotLister, err := backend.MemorizeList(gopts.ctx, repo.Backend(), restic.SnapshotFile)
	if err != nil {
		return err
	}

	if err = repo.LoadIndex(ctx); err != nil {
		return err
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

	for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, statsOptions.Hosts, statsOptions.Tags, statsOptions.Paths, args) {
		err = statsWalkSnapshot(ctx, sn, repo, stats)
		if err != nil {
			return fmt.Errorf("error walking snapshot: %v", err)
		}
	}

	if err != nil {
		return err
	}

	if statsOptions.countMode == countModeRawData {
		// the blob handles have been collected, but not yet counted
		for blobHandle := range stats.blobs {
			pbs := repo.Index().Lookup(blobHandle)
			if len(pbs) == 0 {
				return fmt.Errorf("blob %v not found", blobHandle)
			}
			stats.TotalSize += uint64(pbs[0].Length)
			stats.TotalBlobCount++
		}
	}

	if gopts.JSON {
		err = json.NewEncoder(globalOptions.stdout).Encode(stats)
		if err != nil {
			return fmt.Errorf("encoding output: %v", err)
		}
		return nil
	}

	Printf("Stats in %s mode:\n", statsOptions.countMode)
	Printf("Snapshots processed:   %d\n", stats.SnapshotsCount)

	if stats.TotalBlobCount > 0 {
		Printf("   Total Blob Count:   %d\n", stats.TotalBlobCount)
	}
	if stats.TotalFileCount > 0 {
		Printf("   Total File Count:   %d\n", stats.TotalFileCount)
	}
	Printf("         Total Size:   %-5s\n", formatBytes(stats.TotalSize))

	return nil
}

func statsWalkSnapshot(ctx context.Context, snapshot *restic.Snapshot, repo restic.Repository, stats *statsContainer) error {
	if snapshot.Tree == nil {
		return fmt.Errorf("snapshot %s has nil tree", snapshot.ID().Str())
	}

	stats.SnapshotsCount++

	if statsOptions.countMode == countModeRawData {
		// count just the sizes of unique blobs; we don't need to walk the tree
		// ourselves in this case, since a nifty function does it for us
		return restic.FindUsedBlobs(ctx, repo, restic.IDs{*snapshot.Tree}, stats.blobs, nil)
	}

	uniqueInodes := make(map[uint64]struct{})
	err := walker.Walk(ctx, repo, *snapshot.Tree, restic.NewIDSet(), statsWalkTree(repo, stats, uniqueInodes))
	if err != nil {
		return fmt.Errorf("walking tree %s: %v", *snapshot.Tree, err)
	}

	return nil
}

func statsWalkTree(repo restic.Repository, stats *statsContainer, uniqueInodes map[uint64]struct{}) walker.WalkFunc {
	return func(parentTreeID restic.ID, npath string, node *restic.Node, nodeErr error) (bool, error) {
		if nodeErr != nil {
			return true, nodeErr
		}
		if node == nil {
			return true, nil
		}

		if statsOptions.countMode == countModeUniqueFilesByContents || statsOptions.countMode == countModeBlobsPerFile {
			// only count this file if we haven't visited it before
			fid := makeFileIDByContents(node)
			if _, ok := stats.uniqueFiles[fid]; !ok {
				// mark the file as visited
				stats.uniqueFiles[fid] = struct{}{}

				if statsOptions.countMode == countModeUniqueFilesByContents {
					// simply count the size of each unique file (unique by contents only)
					stats.TotalSize += node.Size
					stats.TotalFileCount++
				}
				if statsOptions.countMode == countModeBlobsPerFile {
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
							blobSize, found := repo.LookupBlobSize(blobID, restic.DataBlob)
							if !found {
								return true, fmt.Errorf("blob %s not found for tree %s", blobID, parentTreeID)
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

		if statsOptions.countMode == countModeRestoreSize {
			// as this is a file in the snapshot, we can simply count its
			// size without worrying about uniqueness, since duplicate files
			// will still be restored
			stats.TotalFileCount++

			// if inodes are present, only count each inode once
			// (hard links do not increase restore size)
			if _, ok := uniqueInodes[node.Inode]; !ok || node.Inode == 0 {
				uniqueInodes[node.Inode] = struct{}{}
				stats.TotalSize += node.Size
			}

			return false, nil
		}

		return true, nil
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

func verifyStatsInput(gopts GlobalOptions, args []string) error {
	// require a recognized counting mode
	switch statsOptions.countMode {
	case countModeRestoreSize:
	case countModeUniqueFilesByContents:
	case countModeBlobsPerFile:
	case countModeRawData:
	default:
		return fmt.Errorf("unknown counting mode: %s (use the -h flag to get a list of supported modes)", statsOptions.countMode)
	}

	return nil
}

// statsContainer holds information during a walk of a repository
// to collect information about it, as well as state needed
// for a successful and efficient walk.
type statsContainer struct {
	TotalSize      uint64 `json:"total_size"`
	TotalFileCount uint64 `json:"total_file_count"`
	TotalBlobCount uint64 `json:"total_blob_count,omitempty"`
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
)
