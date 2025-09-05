package main

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
)

func newPackfileListCommand() *cobra.Command {
	var opts PackfileListOptions

	cmd := &cobra.Command{
		Use:   "packfilelist [flags] [snapshotID [snapshotID]] ...",
		Short: "List the packfiles belonging to a set of snapshots",
		Long: `
The "packfilelist" command lists packfiles belonging to a set of snapshots.
The usual filter options for a snapshotfilter apply: --host, --tag, --path.
Alternatively a snapshot ID, including "latest" or a list of snapshots can
be specified.

If no snapshot list is given, the whole repository is analysed.

Options:
  "--short-id": instead of full packfile ID you will see the first 8 bytes of it
  "--summary":  create a short summary
  "--detail":   can be specified multiple times:
    detail = 0 (default), just the packfile IDs
    detail = 1 packfile ID, type, length in bytes
    detail = 2 packfile ID, type, length in bytes, how many blobs used / total in packfile, size used
  "--orphan": go through all packfiles to find any orphaned packfiles which
              are not referenced and not indexed at all.

  the standard snapshot filter, with --host, --tag and --path. Alternatively specify
  a list of snapshots, including 'latest'.

  "--json": if JSON is specified, the output looks as follows:
   {
    "packfiles": [
			{
				"id": "fe86d10b92eb275860234d1708b6856251bcace1a4dcb4234e26aa24675642dc",
				"type": "data",
				"packfile_size": 14259104,
				"blobs_used_in_snap": 24,
				"blobs_in_packfile": 24,
				"size_used_in_snap": 14258084
			},
      ...
     ],
		"summary": {
			"snap_count": 61,
			"snap_treefile_count": 1,
			"snap_datafile_count": 505,
			"used_blobs_in_snaps": 66616,
			"used_size_in_snaps": 7014631603,
			"used_size_in_packfiles": 7342625876,
			"repository_packfile_count": 506,
			"repository_blob_count": 70777,
			"repository_packfile_size": 8764305665,
			"orphaned_packfile_count": 82,
			"orphaned_blob_count": 3201,
			"orphaned_packfile_size": 1421679789
		}
   }

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
		DisableAutoGenTag: true,
		GroupID:           cmdGroupDefault,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPackfileList(cmd.Context(), opts, globalOptions, args)
		},
	}
	opts.AddFlags(cmd.Flags())

	return cmd
}

// PackfileListOptions collects all options for the packfilelist command.
type PackfileListOptions struct {
	shortID  bool
	detail   int
	summary  bool
	idLength int
	orphaned bool
	noHeader bool
	restic.SnapshotFilter
}

type PFInfo struct {
	blobSize          map[restic.ID]uint         // for each blob which is known
	selectedSnapPacks map[restic.ID]PacklistInfo // per packID
	allSnapPacks      map[restic.ID]PacklistInfo // per packID
}

// PacklistInfo defines one entry per packfile containing the following information
type PacklistInfo struct {
	ID             restic.ID `json:"id"`
	Type           string    `json:"type"`
	Size           int64     `json:"packfile_size"`
	CountUsedBlobs int       `json:"blobs_used_in_snap"`
	CountAllBlobs  int       `json:"blobs_in_packfile"`
	SizeUsed       int64     `json:"size_used_in_snap"`
}

type Summary struct {
	CountSelectedSnaps  int   `json:"snap_count"`
	CountTreeFiles      int   `json:"snap_treefile_count"`
	CountDataFiles      int   `json:"snap_datafile_count"`
	CountUsedPackfiles  int   `json:"active_packfiles_count"`
	UsedBlobsSnapshots  int   `json:"used_blobs_in_snaps"`
	UsedSizeSnapshots   int64 `json:"used_size_in_snaps"`
	UsedSizePackfiles   int64 `json:"used_size_in_packfiles"`
	CountPackfiles      int   `json:"repository_packfile_count"`
	CountBlobsPackfiles int   `json:"repository_blob_count"`
	SizePackfiles       int64 `json:"repository_packfile_size"`
	CountOrphanedPacks  int   `json:"orphaned_packfile_count"`
	CountOrphanedBlobs  int   `json:"orphaned_blob_count"`
	SizeOrphanedPackes  int64 `json:"orphaned_packfile_size"`
}

// outputStruct for JSON
type outputStruct struct {
	PackfileList []PacklistInfo `json:"packfiles"`
	SummaryInfo  Summary        `json:"summary"`
}

func (opts *PackfileListOptions) AddFlags(f *pflag.FlagSet) {
	f.BoolVarP(&opts.summary, "summary", "S", false, "show summary")
	f.CountVarP(&opts.detail, "detail", "D", "some/more detail information of packfile usage")
	f.BoolVarP(&opts.shortID, "short-id", "s", false, "short packfile ID instead of full ID")
	f.BoolVarP(&opts.orphaned, "orphan", "O", false, "check all packfiles manually")
	f.BoolVarP(&opts.noHeader, "no-header", "N", false, "print no header for output")
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)
}

// CheckWithSnapshots will process snapshot IDs from 'selectedTrees'
func (pfInfo *PFInfo) CheckWithSnapshots(ctx context.Context, repo *repository.Repository,
	selectedTrees []restic.ID, gopts GlobalOptions, orphaned bool) error {
	var err error

	// gather used blobs from all trees in 'selectedTrees'
	usedBlobs := restic.NewBlobSet()
	bar := newIndexProgress(gopts.Quiet, gopts.JSON)
	if err = restic.FindUsedBlobs(ctx, repo, selectedTrees, usedBlobs, bar); err != nil {
		return err
	}

	// get length of packfiles from repository via index
	repoPacks, err := pack.Size(ctx, repo, false)
	if err != nil {
		return err
	}

	// get information about all indexed blobs and packfiles
	err = repo.ListBlobs(ctx, func(packedBlob restic.PackedBlob) {
		packID := packedBlob.PackID
		if old, ok := pfInfo.allSnapPacks[packedBlob.PackID]; !ok {
			pfInfo.allSnapPacks[packID] = PacklistInfo{
				ID:            packID,
				Type:          packedBlob.Type.String(),
				Size:          repoPacks[packedBlob.PackID],
				CountAllBlobs: 1,
			}
		} else {
			pfInfo.allSnapPacks[packID] = PacklistInfo{
				ID:            packID,
				Type:          old.Type,
				Size:          old.Size,
				CountAllBlobs: old.CountAllBlobs + 1,
			}
		}
		pfInfo.blobSize[packedBlob.ID] = packedBlob.Length
	})
	if err != nil {
		return err
	}

	// if requested check all packfiles for unreferenced blobs
	if orphaned {
		err = repo.List(ctx, restic.PackFile, func(id restic.ID, size int64) error {
			if _, ok := pfInfo.allSnapPacks[id]; !ok {
				// get info from packfile directly
				blobs, _, err := repo.ListPack(ctx, id, size)
				if err != nil {
					return err
				}
				Type := "????"
				if len(blobs) > 0 {
					Type = blobs[0].Type.String()
				}
				pfInfo.allSnapPacks[id] = PacklistInfo{
					ID:            id,
					Type:          Type,
					Size:          size,
					CountAllBlobs: len(blobs),
				}
				pfInfo.selectedSnapPacks[id] = PacklistInfo{
					ID:            id,
					Type:          Type,
					Size:          size,
					CountAllBlobs: len(blobs),
				}
				for _, blob := range blobs {
					pfInfo.blobSize[blob.ID] = blob.Length
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	// convert used blobs to packfile IDs and collect statistics
	for blobHandle := range usedBlobs {
		for _, blobInner := range repo.LookupBlob(blobHandle.Type, blobHandle.ID) {
			if old, ok := pfInfo.selectedSnapPacks[blobInner.PackID]; !ok {
				pfInfo.selectedSnapPacks[blobInner.PackID] = PacklistInfo{
					ID:             blobInner.PackID,
					Type:           blobInner.Type.String(),
					Size:           repoPacks[blobInner.PackID],
					CountUsedBlobs: 1,
					CountAllBlobs:  pfInfo.allSnapPacks[blobInner.PackID].CountAllBlobs,
					SizeUsed:       int64(pfInfo.blobSize[blobHandle.ID]),
				}
			} else {
				pfInfo.selectedSnapPacks[blobInner.PackID] = PacklistInfo{
					ID:             old.ID,
					Type:           old.Type,
					Size:           old.Size,
					CountAllBlobs:  old.CountAllBlobs,
					CountUsedBlobs: old.CountUsedBlobs + 1,
					SizeUsed:       old.SizeUsed + int64(pfInfo.blobSize[blobHandle.ID]),
				}
			}
		}
	}

	return nil
}

// runPackfileList runs the command 'packfilelist'
func runPackfileList(ctx context.Context, opts PackfileListOptions, gopts GlobalOptions, args []string) error {
	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, true)
	if err != nil {
		return err
	}
	defer unlock()

	selectedTrees := make([]restic.ID, 0, 100)
	snapshotLister, err := restic.MemorizeList(ctx, repo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	// index needs to be loaded
	if err = repo.LoadIndex(ctx, newIndexProgress(gopts.Quiet, gopts.JSON)); err != nil {
		return err
	}

	// find all selected snapshots
	err = (&opts.SnapshotFilter).FindAll(ctx, snapshotLister, repo, args, func(_ string, sn *restic.Snapshot, err error) error {
		if err != nil {
			return err
		}

		selectedTrees = append(selectedTrees, *sn.Tree)
		return nil
	})
	if err != nil {
		return err
	} else if len(selectedTrees) == 0 {
		return errors.Fatal("snapshotfilter active but no snapshot selected")
	}

	// gather active packfiles list
	pfInfo := &PFInfo{
		blobSize:          make(map[restic.ID]uint),
		selectedSnapPacks: make(map[restic.ID]PacklistInfo),
		allSnapPacks:      make(map[restic.ID]PacklistInfo),
	}
	if err = pfInfo.CheckWithSnapshots(ctx, repo, selectedTrees, gopts, opts.orphaned); err != nil {
		return err
	}

	// sort packfile IDs
	packfilesSort := make([]restic.ID, 0, len(pfInfo.selectedSnapPacks))
	for packfileID := range pfInfo.selectedSnapPacks {
		packfilesSort = append(packfilesSort, packfileID)
	}
	slices.SortStableFunc(packfilesSort, func(a, b restic.ID) int {
		return bytes.Compare(a[:], b[:])
	})

	// count and size
	typeCount := make(map[string]int, 2)
	repositorySize := int64(0)
	selectedPackfileSize := int64(0)
	snapSizeUsed := int64(0)
	sizeOrphaned := int64(0)
	countAllBlobs := 0
	countUsedBlobs := 0
	countOrphaned := 0
	countOrphanedBlobs := 0
	countUsedPackfiles := 0

	for _, d := range pfInfo.selectedSnapPacks {
		if d.CountUsedBlobs == 0 {
			countOrphaned++
			countOrphanedBlobs += d.CountAllBlobs
			sizeOrphaned += d.Size
		} else {
			selectedPackfileSize += d.Size
			countUsedPackfiles++
		}
		snapSizeUsed += int64(d.SizeUsed)
		countUsedBlobs += d.CountUsedBlobs
		typeCount[d.Type]++
	}

	for id, d := range pfInfo.allSnapPacks {
		repositorySize += d.Size
		countAllBlobs += d.CountAllBlobs

		if _, ok := pfInfo.selectedSnapPacks[id]; !ok {
			countOrphaned++
			countOrphanedBlobs += d.CountAllBlobs
			sizeOrphaned += d.Size
		}
	}

	summary := Summary{
		CountSelectedSnaps:  len(selectedTrees),
		CountTreeFiles:      typeCount["tree"],
		CountDataFiles:      typeCount["data"],
		CountUsedPackfiles:  countUsedPackfiles,
		UsedBlobsSnapshots:  countUsedBlobs,
		UsedSizeSnapshots:   snapSizeUsed,
		UsedSizePackfiles:   selectedPackfileSize,
		CountPackfiles:      len(pfInfo.allSnapPacks),
		CountBlobsPackfiles: countAllBlobs,
		SizePackfiles:       repositorySize,
		CountOrphanedPacks:  countOrphaned,
		CountOrphanedBlobs:  countOrphanedBlobs,
		SizeOrphanedPackes:  sizeOrphaned,
	}

	snapshotFilterActive := !opts.SnapshotFilter.Empty() || len(args) > 0
	if gopts.JSON {
		result, err := produceJSONOutput(packfilesSort, pfInfo.selectedSnapPacks, summary, snapshotFilterActive)
		if err != nil {
			return err
		}
		Println(result)
	}

	if opts.detail > 2 {
		opts.detail = 2
	}

	opts.idLength = 64
	if opts.shortID {
		opts.idLength = 8
	}

	// print header
	if !gopts.Quiet && !opts.noHeader {
		switch opts.detail {
		case 0:
			Println("packfile")
			if opts.shortID {
				Println("========")
			} else {
				Println(strings.Repeat("=", 64))
			}
		case 1:
			Printf("%-*s %4s %10s\n", opts.idLength, "packfile", "type", "length")
			Println(strings.Repeat("=", opts.idLength+1+4+1+10))
		case 2:
			Printf("%-*s %4s %10s  %5s    %5s %10s\n", opts.idLength, "packfile", "type", "length", "used", "count", "length use")
			Println(strings.Repeat("=", opts.idLength+1+4+1+10+2+5+4+5+1+10))
		}
	}

	// print packfile info
	for _, packfileID := range packfilesSort {
		d := pfInfo.selectedSnapPacks[packfileID]

		printID := packfileID.String()
		if opts.shortID {
			printID = printID[:8]
		}
		if !gopts.Quiet {
			switch opts.detail {
			case 0:
				Printf("%s\n", printID)
			case 1:
				Printf("%s %4s %10d\n", printID, d.Type, d.Size)
			case 2:
				Printf("%s %4s %10d  %5d of %5d %10d\n", printID, d.Type, d.Size,
					d.CountUsedBlobs, d.CountAllBlobs, d.SizeUsed)
			}
		}
	}

	// print summary
	if opts.summary {
		Println()
		Printf("\nnumber of selected snaps %11d\n", len(selectedTrees))
		Printf("tree packfiles for snaps %11d\n", typeCount["tree"])
		Printf("data packfiles for snaps %11d\n", typeCount["data"])

		Printf("\nactive packfiles         %11d\n", countUsedPackfiles)
		Printf("used blobs in snapshots  %11d\n", countUsedBlobs)
		Printf("used size of  snapshots  %11s\n", ui.FormatBytes(uint64(snapSizeUsed)))
		Printf("size selected packfiles  %11s\n", ui.FormatBytes(uint64(selectedPackfileSize)))

		Printf("\ncount all packfiles      %11d\n", len(pfInfo.allSnapPacks))
		Printf("all blobs in repository  %11d\n", countAllBlobs)
		Printf("size all packfiles       %11s\n", ui.FormatBytes(uint64(repositorySize)))

		if !snapshotFilterActive {
			if countAllBlobs != countUsedBlobs {
				Printf("\ncount unused blobs       %11d\n", countAllBlobs-countUsedBlobs)
				Printf("size  unused blobs       %11s\n", ui.FormatBytes(uint64(repositorySize-snapSizeUsed)))
			}
			if sizeOrphaned > 0 {
				Printf("\ncount orphaned packfiles %11d\n", countOrphaned)
				Printf("count orphaned blobs     %11d\n", countOrphanedBlobs)
				Printf("size  orphaned packfiles %11s\n", ui.FormatBytes(uint64(sizeOrphaned)))
			}
		}
	}

	return nil
}

// produceJSONOutput generates JSON output
func produceJSONOutput(packfiles []restic.ID, selectedSnapPacks map[restic.ID]PacklistInfo,
	summary Summary, snapshotFilterActive bool) (string, error) {

	// result JSON struct: all packfile info plus a summary
	var output outputStruct

	output.SummaryInfo = summary
	output.PackfileList = make([]PacklistInfo, 0, len(packfiles))
	for _, packfileID := range packfiles {
		d := selectedSnapPacks[packfileID]
		if snapshotFilterActive && d.CountUsedBlobs == 0 {
			continue
		}
		output.PackfileList = append(output.PackfileList, d)
	}

	buf, err := json.Marshal(output)

	return string(buf), err
}
