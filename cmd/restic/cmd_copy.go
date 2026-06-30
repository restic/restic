package main

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"slices"
	"sync"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newCopyCommand(globalOptions *global.Options) *cobra.Command {
	var opts CopyOptions
	cmd := &cobra.Command{
		Use:   "copy [flags] [snapshotID ...]",
		Short: "Copy snapshots from one repository to another",
		Long: `
The "copy" command copies one or more snapshots from one repository to another.

NOTE: This process will have to both download (read) and upload (write) the
entire snapshot(s) due to the different encryption keys used in the source and
destination repositories. This /may incur higher bandwidth usage and costs/ than
expected during normal backup runs.

NOTE: The copying process does not re-chunk files, which may break deduplication
between the files copied and files already stored in the destination repository.
This means that copied files, which existed in both the source and destination
repository, /may occupy up to twice their space/ in the destination repository.
This can be mitigated by the "--copy-chunker-params" option when initializing a
new destination repository using the "init" command.

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
			return runCopy(cmd.Context(), opts, *globalOptions, args, globalOptions.Term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

// CopyOptions bundles all options for the copy command.
type CopyOptions struct {
	global.SecondaryRepoOptions
	data.SnapshotFilter
}

func (opts *CopyOptions) AddFlags(f *pflag.FlagSet) {
	opts.SecondaryRepoOptions.AddFlags(f, "destination", "to copy snapshots from")
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)
}

var errSentinelEndIteration = errors.New("end iteration")

// collectAllSnapshots: select all snapshot trees to be copied
func collectAllSnapshots(ctx context.Context, opts CopyOptions,
	srcSnapshotLister restic.Lister, srcRepo restic.Repository,
	dstSnapshotByOriginal map[restic.ID][]*data.Snapshot, args []string, printer restic.Printer,
) iter.Seq2[*data.Snapshot, error] {
	return func(yield func(*data.Snapshot, error) bool) {
		err := opts.SnapshotFilter.FindAll(ctx, srcSnapshotLister, srcRepo, args, func(_ string, sn *data.Snapshot, err error) error {
			// check whether the destination has a snapshot with the same persistent ID which has similar snapshot fields
			if err != nil {
				if !yield(nil, err) {
					return errSentinelEndIteration
				}
				return nil
			}
			srcOriginal := *sn.ID()
			if sn.Original != nil {
				srcOriginal = *sn.Original
			}
			if originalSns, ok := dstSnapshotByOriginal[srcOriginal]; ok {
				isCopy := false
				for _, originalSn := range originalSns {
					if similarSnapshots(originalSn, sn) {
						printer.V("\n%v", sn)
						printer.V("skipping source snapshot %s, was already copied to snapshot %s", sn.ID().Str(), originalSn.ID().Str())
						isCopy = true
						break
					}
				}
				if isCopy {
					return nil
				}
			}
			if !yield(sn, nil) {
				return errSentinelEndIteration
			}
			return nil
		})
		if err != nil && !errors.Is(err, errSentinelEndIteration) {
			yield(nil, err)
		}
	}
}

func runCopy(ctx context.Context, opts CopyOptions, gopts global.Options, args []string, term ui.Terminal) error {
	printer := progress.NewTerminalPrinter(gopts.JSON, gopts.Verbosity, term)
	secondaryGopts, isFromRepo, err := opts.SecondaryRepoOptions.FillGlobalOpts(ctx, gopts, "destination")
	if err != nil {
		return err
	}
	if isFromRepo {
		// swap global options, if the secondary repo was set via from-repo
		gopts, secondaryGopts = secondaryGopts, gopts
	}

	ctx, srcRepo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	ctx, dstRepo, unlock, err := openWithAppendLock(ctx, secondaryGopts, false, printer)
	if err != nil {
		return err
	}
	defer unlock()

	srcSnapshotLister, err := restic.MemorizeList(ctx, srcRepo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	dstSnapshotLister, err := restic.MemorizeList(ctx, dstRepo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	debug.Log("Loading source index")
	if err := srcRepo.LoadIndex(ctx, printer); err != nil {
		return err
	}
	debug.Log("Loading destination index")
	if err := dstRepo.LoadIndex(ctx, printer); err != nil {
		return err
	}

	dstSnapshotByOriginal := make(map[restic.ID][]*data.Snapshot)
	err = opts.SnapshotFilter.FindAll(ctx, dstSnapshotLister, dstRepo, nil, func(_ string, sn *data.Snapshot, err error) error {
		if err != nil {
			return err
		}
		if sn.Original != nil && !sn.Original.IsNull() {
			dstSnapshotByOriginal[*sn.Original] = append(dstSnapshotByOriginal[*sn.Original], sn)
		}
		// also consider identical snapshot copies
		dstSnapshotByOriginal[*sn.ID()] = append(dstSnapshotByOriginal[*sn.ID()], sn)
		return nil
	})
	if err != nil {
		return err
	}

	selectedSnapshots := collectAllSnapshots(ctx, opts, srcSnapshotLister, srcRepo, dstSnapshotByOriginal, args, printer)

	if err := copyTreeBatched(ctx, gopts, srcRepo, dstRepo, selectedSnapshots, printer); err != nil {
		return err
	}

	return ctx.Err()
}

func similarSnapshots(sna *data.Snapshot, snb *data.Snapshot) bool {
	// everything except Parent and Original must match
	if !sna.Time.Equal(snb.Time) || !sna.Tree.Equal(*snb.Tree) || sna.Hostname != snb.Hostname ||
		sna.Username != snb.Username || sna.UID != snb.UID || sna.GID != snb.GID ||
		len(sna.Paths) != len(snb.Paths) || len(sna.Excludes) != len(snb.Excludes) ||
		len(sna.Tags) != len(snb.Tags) {
		return false
	}
	if !sna.HasPaths(snb.Paths) || !sna.HasTags(snb.Tags) {
		return false
	}
	for i, a := range sna.Excludes {
		if a != snb.Excludes[i] {
			return false
		}
	}
	return true
}

type CopyDataJSON struct {
	SourceSnapshotID restic.ID `json:"source_id"`
	//SourceShortID       string         `json:"source_short_id"`
	TargetSnapshotID restic.ID `json:"target_id"`
	//TargetShortID       string         `json:"target_short_id"`
	SrcSnapshot         *data.Snapshot `json:"source_snapshot"`
	CountBlobs          int            `json:"count_blobs"`
	SizeBlobs           uint64         `json:"size_blobs"`
	TotalFilesProcessed uint           `json:"total_files_processed"`
	TotalBytesProcessed uint64         `json:"total_bytes_processed"`
}

// copyTreeBatched copies multiple snapshots in one go. Snapshots are written after
// data equivalent to at least 10 packfiles was written.
func copyTreeBatched(ctx context.Context, gopts global.Options, srcRepo *repository.Repository, dstRepo restic.Repository,
	selectedSnapshots iter.Seq2[*data.Snapshot, error], printer restic.Printer) error {

	// remember already processed trees across all snapshots
	visitedTrees := srcRepo.NewAssociatedBlobSet()
	copyStatisticsJSON := make(map[*data.Snapshot]CopyDataJSON)
	targetSize := uint64(dstRepo.PackSize()) * 100
	minDuration := 1 * time.Minute

	// use pull-based iterator to allow iteration in multiple steps
	next, stop := iter.Pull2(selectedSnapshots)
	defer stop()

	for {
		var batch []*data.Snapshot
		batchSize := uint64(0)
		startTime := time.Now()

		// call WithBlobUploader() once and then loop over all selectedSnapshots
		err := dstRepo.WithBlobUploader(ctx, func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
			for batchSize < targetSize || time.Since(startTime) < minDuration {
				sn, err, ok := next()
				if err != nil {
					return err
				}
				if !ok {
					break
				}

				batch = append(batch, sn)

				printer.P("\n%v", sn)
				printer.P("  copy started, this may take a while...")
				copyStatisticsEntry, err := copyTree(ctx, srcRepo, dstRepo, visitedTrees, *sn.Tree, printer, uploader)
				if err != nil {
					return err
				}
				debug.Log("tree copied")
				temp := CopyDataJSON{
					SrcSnapshot:      sn,
					SourceSnapshotID: *sn.ID(),
					//SourceShortID:       sn.ID().Str(),
					CountBlobs: copyStatisticsEntry.CountBlobs,
					SizeBlobs:  copyStatisticsEntry.SizeBlobs,
				}
				if sn.Summary != nil {
					temp.TotalFilesProcessed = sn.Summary.TotalFilesProcessed
					temp.TotalBytesProcessed = sn.Summary.TotalBytesProcessed
				}
				// suppress sn.Summary
				temp.SrcSnapshot.Summary = nil
				copyStatisticsJSON[sn] = temp
				batchSize += copyStatisticsEntry.SizeBlobs
			}

			return nil
		})
		if err != nil {
			return err
		}

		// if no snapshots were processed in this batch, we're done
		if len(batch) == 0 {
			break
		}

		// add a newline to separate saved snapshot messages from the other messages
		if len(batch) > {
			printer.P("")
		}
		// save all the snapshots
		for _, sn := range batch {
			temp := copyStatisticsJSON[sn]
			err := copySaveSnapshot(ctx, sn, dstRepo, printer, &temp)
			if err != nil {
				return err
			}
			copyStatisticsJSON[sn] = temp
		}
	}

	if gopts.JSON {
		copyStatisticsJSONSort := make([]CopyDataJSON, 0, len(copyStatisticsJSON))
		for _, data := range copyStatisticsJSON {
			copyStatisticsJSONSort = append(copyStatisticsJSONSort, data)
		}
		slices.SortFunc(copyStatisticsJSONSort, func(a, b CopyDataJSON) int {
			return a.SrcSnapshot.Time.Compare(b.SrcSnapshot.Time)
		})

		err := json.NewEncoder(gopts.Term.OutputWriter()).Encode(copyStatisticsJSONSort)
		return err
	}

	return nil
}

func copyTree(
	ctx context.Context,
	srcRepo *repository.Repository,
	dstRepo restic.Repository,
	visitedTrees restic.AssociatedBlobSet,
	rootTreeID restic.ID,
	printer restic.Printer,
	uploader restic.BlobSaverWithAsync,
) (CopyDataJSON, error) {

	copyBlobs := srcRepo.NewAssociatedBlobSet()
	packList := restic.NewIDSet()
	var lock sync.Mutex

	enqueue := func(h restic.BlobHandle) {
		lock.Lock()
		defer lock.Unlock()
		if _, ok := dstRepo.LookupBlobSize(h); !ok {
			pb := srcRepo.LookupBlob(h)
			copyBlobs.Insert(h)
			for _, p := range pb {
				packList.Insert(p.PackID())
			}
		}
	}

	err := data.StreamTrees(ctx, srcRepo, restic.IDs{rootTreeID}, restic.NoopCounter, func(treeID restic.ID) bool {
		handle := restic.BlobHandle{ID: treeID, Type: restic.TreeBlob}
		visited := visitedTrees.Has(handle)
		visitedTrees.Insert(handle)
		return visited
	}, func(treeID restic.ID, err error, nodes data.TreeNodeIterator) error {
		if err != nil {
			return fmt.Errorf("LoadTree(%v) returned error %v", treeID.Str(), err)
		}

		// copy raw tree bytes to avoid problems if the serialization changes
		enqueue(restic.BlobHandle{ID: treeID, Type: restic.TreeBlob})

		for item := range nodes {
			if item.Error != nil {
				return item.Error
			}
			// Recursion into directories is handled by StreamTrees
			// Copy the blobs for this file.
			for _, blobID := range item.Node.Content {
				enqueue(restic.BlobHandle{Type: restic.DataBlob, ID: blobID})
			}
		}
		return nil
	})
	if err != nil {
		return CopyDataJSON{}, err
	}

	copyStatisticsEntry := copyStats(srcRepo, copyBlobs, packList, printer)
	bar := printer.NewCounter("packs copied")
	err = repository.CopyBlobs(ctx, srcRepo, dstRepo, uploader, packList, copyBlobs, bar, printer.P)
	if err != nil {
		return CopyDataJSON{}, errors.Fatalf("%s", err)
	}
	return copyStatisticsEntry, nil
}

// copyStats: print statistics for the blobs to be copied
func copyStats(srcRepo restic.Repository, copyBlobs restic.AssociatedBlobSet, packList restic.IDSet, printer restic.Printer) CopyDataJSON {
	// count and size
	countBlobs := 0
	sizeBlobs := uint64(0)
	result := CopyDataJSON{}
	for blob := range copyBlobs.Keys() {
		for _, pb := range srcRepo.LookupBlob(blob) {
			countBlobs++
			sizeBlobs += uint64(pb.CiphertextLength())
			break
		}
	}
	result.CountBlobs = countBlobs
	result.SizeBlobs = sizeBlobs

	printer.V("  copy %d blobs with disk size %s in %d packfiles\n",
		countBlobs, ui.FormatBytes(uint64(sizeBlobs)), len(packList))
	return result
}

func copySaveSnapshot(ctx context.Context, sn *data.Snapshot, dstRepo restic.Repository, printer restic.Printer, copyStatisticsEntry *CopyDataJSON) error {
	sn.Parent = nil // Parent does not have relevance in the new repo.
	// Use Original as a persistent snapshot ID
	if sn.Original == nil {
		sn.Original = sn.ID()
	}
	newID, err := data.SaveSnapshot(ctx, dstRepo, sn)
	if err != nil {
		return err
	}
	printer.P("snapshot %s saved, copied from source snapshot %s", newID.Str(), sn.ID().Str())
	copyStatisticsEntry.TargetSnapshotID = newID
	//copyStatisticsEntry.TargetShortID = newID.Str()
	return nil
}
