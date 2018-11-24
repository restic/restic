package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
	"github.com/spf13/cobra"
)

var cmdStats = &cobra.Command{
	Use:   "stats [flags] [snapshot-ID]",
	Short: "Scan the repository and show basic statistics",
	Long: `
The "stats" command walks one or all snapshots in a repository and
accumulates statistics about the data stored therein. It reports on
the number of unique files and their sizes, according to one of
the counting modes as given by the --mode flag.

If no snapshot is specified, all snapshots will be considered. Some
modes make more sense over just a single snapshot, while others
are useful across all snapshots, depending on what you are trying
to calculate.

The modes are:

* restore-size: (default) Counts the size of the restored files.
* files-by-contents: Counts total size of files, where a file is
   considered unique if it has unique contents.
* raw-data: Counts the size of blobs in the repository, regardless of
  how many files reference them.
* blobs-per-file: A combination of files-by-contents and raw-data.
* Refer to the online manual for more details about each mode.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStats(globalOptions, args)
	},
}

func init() {
	cmdRoot.AddCommand(cmdStats)
	f := cmdStats.Flags()
	f.StringVar(&countMode, "mode", countModeRestoreSize, "counting mode: restore-size (default), files-by-contents, blobs-per-file, or raw-data")
	f.StringVarP(&snapshotByHost, "host", "H", "", "filter latest snapshot by this hostname")
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

	if err = repo.LoadIndex(ctx); err != nil {
		return err
	}

	if !gopts.NoLock {
		lock, err := lockRepo(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	if !gopts.JSON {
		Printf("scanning...\n")
	}

	// create a container for the stats (and other needed state)
	stats := &statsContainer{
		uniqueFiles: make(map[fileID]struct{}),
		fileBlobs:   make(map[string]restic.IDSet),
		blobs:       restic.NewBlobSet(),
		blobsSeen:   restic.NewBlobSet(),
	}

	if snapshotIDString != "" {
		// scan just a single snapshot

		var sID restic.ID
		if snapshotIDString == "latest" {
			sID, err = restic.FindLatestSnapshot(ctx, repo, []string{}, []restic.TagList{}, snapshotByHost)
			if err != nil {
				return errors.Fatalf("latest snapshot for criteria not found: %v", err)
			}
		} else {
			sID, err = restic.FindSnapshot(repo, snapshotIDString)
			if err != nil {
				return errors.Fatalf("error loading snapshot: %v", err)
			}
		}

		snapshot, err := restic.LoadSnapshot(ctx, repo, sID)
		if err != nil {
			return errors.Fatalf("error loading snapshot from repo: %v", err)
		}

		err = statsWalkSnapshot(ctx, snapshot, repo, stats)
	} else {
		// iterate every snapshot in the repo
		err = repo.List(ctx, restic.SnapshotFile, func(snapshotID restic.ID, size int64) error {
			snapshot, err := restic.LoadSnapshot(ctx, repo, snapshotID)
			if err != nil {
				return fmt.Errorf("Error loading snapshot %s: %v", snapshotID.Str(), err)
			}
			return statsWalkSnapshot(ctx, snapshot, repo, stats)
		})
	}
	if err != nil {
		return err
	}

	if countMode == countModeRawData {
		// the blob handles have been collected, but not yet counted
		for blobHandle := range stats.blobs {
			blobSize, found := repo.LookupBlobSize(blobHandle.ID, blobHandle.Type)
			if !found {
				return fmt.Errorf("blob %v not found", blobHandle)
			}
			stats.TotalSize += uint64(blobSize)
			stats.TotalBlobCount++
		}
	}

	if gopts.JSON {
		err = json.NewEncoder(os.Stdout).Encode(stats)
		if err != nil {
			return fmt.Errorf("encoding output: %v", err)
		}
		return nil
	}

	// inform the user what was scanned and how it was scanned
	snapshotsScanned := snapshotIDString
	if snapshotsScanned == "latest" {
		snapshotsScanned = "the latest snapshot"
	} else if snapshotsScanned == "" {
		snapshotsScanned = "all snapshots"
	}
	Printf("Stats for %s in %s mode:\n", snapshotsScanned, countMode)

	if stats.TotalBlobCount > 0 {
		Printf("  Total Blob Count:   %d\n", stats.TotalBlobCount)
	}
	if stats.TotalFileCount > 0 {
		Printf("  Total File Count:   %d\n", stats.TotalFileCount)
	}
	Printf("        Total Size:   %-5s\n", formatBytes(stats.TotalSize))

	return nil
}

func statsWalkSnapshot(ctx context.Context, snapshot *restic.Snapshot, repo restic.Repository, stats *statsContainer) error {
	if snapshot.Tree == nil {
		return fmt.Errorf("snapshot %s has nil tree", snapshot.ID().Str())
	}

	if countMode == countModeRawData {
		// count just the sizes of unique blobs; we don't need to walk the tree
		// ourselves in this case, since a nifty function does it for us
		return restic.FindUsedBlobs(ctx, repo, *snapshot.Tree, stats.blobs, stats.blobsSeen)
	}

	err := walker.Walk(ctx, repo, *snapshot.Tree, restic.NewIDSet(), statsWalkTree(repo, stats))
	if err != nil {
		return fmt.Errorf("walking tree %s: %v", *snapshot.Tree, err)
	}
	return nil
}

func statsWalkTree(repo restic.Repository, stats *statsContainer) walker.WalkFunc {
	return func(_ restic.ID, npath string, node *restic.Node, nodeErr error) (bool, error) {
		if nodeErr != nil {
			return true, nodeErr
		}
		if node == nil {
			return true, nil
		}

		if countMode == countModeUniqueFilesByContents || countMode == countModeBlobsPerFile {
			// only count this file if we haven't visited it before
			fid := makeFileIDByContents(node)
			if _, ok := stats.uniqueFiles[fid]; !ok {
				// mark the file as visited
				stats.uniqueFiles[fid] = struct{}{}

				if countMode == countModeUniqueFilesByContents {
					// simply count the size of each unique file (unique by contents only)
					stats.TotalSize += node.Size
					stats.TotalFileCount++
				}
				if countMode == countModeBlobsPerFile {
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
								return true, fmt.Errorf("blob %s not found for tree %s", blobID, *node.Subtree)
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

		if countMode == countModeRestoreSize {
			// as this is a file in the snapshot, we can simply count its
			// size without worrying about uniqueness, since duplicate files
			// will still be restored
			stats.TotalSize += node.Size
			stats.TotalFileCount++
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
	switch countMode {
	case countModeRestoreSize:
	case countModeUniqueFilesByContents:
	case countModeBlobsPerFile:
	case countModeRawData:
	default:
		return fmt.Errorf("unknown counting mode: %s (use the -h flag to get a list of supported modes)", countMode)
	}

	// ensure at most one snapshot was specified
	if len(args) > 1 {
		return fmt.Errorf("only one snapshot may be specified")
	}

	// if a snapshot was specified, mark it as the one to scan
	if len(args) == 1 {
		snapshotIDString = args[0]
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

	// uniqueFiles marks visited files according to their
	// contents (hashed sequence of content blob IDs)
	uniqueFiles map[fileID]struct{}

	// fileBlobs maps a file name (path) to the set of
	// blobs that have been seen as a part of the file
	fileBlobs map[string]restic.IDSet

	// blobs and blobsSeen are used to count individual
	// unique blobs, independent of references to files
	blobs, blobsSeen restic.BlobSet
}

// fileID is a 256-bit hash that distinguishes unique files.
type fileID [32]byte

var (
	// the mode of counting to perform
	countMode string

	// the snapshot to scan, as given by the user
	snapshotIDString string

	// snapshotByHost is the host to filter latest
	// snapshot by, if given by user
	snapshotByHost string
)

const (
	countModeRestoreSize           = "restore-size"
	countModeUniqueFilesByContents = "files-by-contents"
	countModeBlobsPerFile          = "blobs-per-file"
	countModeRawData               = "raw-data"
)
