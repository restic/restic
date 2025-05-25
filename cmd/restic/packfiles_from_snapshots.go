package main

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"

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
		Use:   "packfilelist [flags] snapshotID ",
		Short: "List the packfiles belonging to a set of snapshots",
		Long: `
The "packfilelist" command lists packfiles belonging to a set of snapshots.
The usual filter options for a snapshotfilter apply: --host, --tag, --path.
Alternatively a snapshot ID, including "latest" or a list of snapshots can
be specified.

Options:
	"--short-id": instead of full packfile ID you will see the first 8 bytes of it
	"--detail", can be specified multiple times:
		detail = 0 (default), just the packfile ID
		detail = 1 packfile ID, type, length in bytes
		detail = 2 packfile ID, type, length in bytes, how many blobs used / total in packfile
		detail = 3 packfile ID, type, length in bytes, how many blobs used / total in packfile, size used
	the standard snapshot filter, with --host, --tag and --path. Alternatively specify
	a list of snapshots, including 'latest'.
	"--json". If JSON is specified, the output looks as follows:
	 {
		"PackfileList": [
			{
			 "id": "8992842f...",
			 "type": "data",
			 "packfile_size": 81873,
			 "blobs_used_in_snap": 69,
			 "blobs_in_packfile": 69,
			 "size_used_in_snap": 79008
			},
			...
		 ],
		 "Summary": {
			"snap_treefiles": 1,
			"snap_datafiles": 1,
			"snap_size_used": 82229, // actual size of snap (compressed)
			"snap_size": 85171, // all packfiles used by this snap
			"repo_size": 85171, // total of all packfiles in repo
			"repo_packfiles": 2
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
	restic.SnapshotFilter
	shortID bool
	detail  int
}

// PacklistInfo defines one entry per packfile containing the following
// information
type PacklistInfo struct {
	ID             restic.ID `json:"id"`
	Type           string    `json:"type"`
	Size           int64     `json:"packfile_size"`
	CountUsedBlobs int       `json:"blobs_used_in_snap"`
	CountAllBlobs  int       `json:"blobs_in_packfile"`
	SizeUsed       int64     `json:"size_used_in_snap"`
}

// outputStruct for JSON
type outputStruct struct {
	PackfileList []PacklistInfo
	Summary      struct {
		CountTreeFiles   int   `json:"snap_treefiles"`
		CountDataFiles   int   `json:"snap_datafiles"`
		SizeSnapshotUsed int64 `json:"snap_size_used"`
		SizeSnapshot     int64 `json:"snap_size"`
		SizeRepo         int64 `json:"repo_size"`
		CountPackfiles   int   `json:"repo_packfiles"`
	}
}

func (opts *PackfileListOptions) AddFlags(f *pflag.FlagSet) {
	initMultiSnapshotFilter(f, &opts.SnapshotFilter, true)
	f.CountVarP(&opts.detail, "detail", "d", "some/more datail information of packfiles usage")
	f.BoolVarP(&opts.shortID, "short-id", "s", false, "sohort ID instead of full ID")
}

// CheckWithSnapshots will process snapshot IDs from 'selectedTrees'
func CheckWithSnapshots(ctx context.Context, repo *repository.Repository,
	selectedTrees []restic.ID, gopts GlobalOptions) (map[restic.ID]PacklistInfo, error) {
	// get length of packs from repository
	repoPacks, err := pack.Size(ctx, repo, false)
	if err != nil {
		return nil, err
	}

	// gather used blobs from all trees in 'selectedTrees'
	usedBlobs := restic.NewBlobSet()
	bar := newIndexProgress(gopts.Quiet, gopts.JSON)
	err = restic.FindUsedBlobs(ctx, repo, selectedTrees, usedBlobs, bar)
	if err != nil {
		return nil, err
	}

	blobsPerPackfile := make(map[restic.ID]int)
	blobSize := make(map[restic.ID]uint)
	err = repo.ListBlobs(ctx, func(blob restic.PackedBlob) {
		blobsPerPackfile[blob.PackID]++
		blobSize[blob.ID] = blob.Length
	})
	if err != nil {
		return nil, err
	}

	// convert blobs to packfile IDs
	snapPacks := make(map[restic.ID]PacklistInfo)
	for blob := range usedBlobs {
		for _, res := range repo.LookupBlob(blob.Type, blob.ID) {
			if old, ok := snapPacks[res.PackID]; !ok {
				snapPacks[res.PackID] = PacklistInfo{
					ID:             res.PackID,
					Type:           res.Type.String(),
					Size:           repoPacks[res.PackID],
					CountUsedBlobs: 1,
					CountAllBlobs:  blobsPerPackfile[res.PackID],
					SizeUsed:       int64(blobSize[blob.ID]),
				}
			} else {
				snapPacks[res.PackID] = PacklistInfo{
					ID:             res.PackID,
					Type:           res.Type.String(),
					Size:           old.Size,
					CountAllBlobs:  blobsPerPackfile[res.PackID],
					CountUsedBlobs: old.CountUsedBlobs + 1,
					SizeUsed:       old.SizeUsed + int64(blobSize[blob.ID]),
				}
			}
		}
	}

	return snapPacks, nil
}

// runPackfileList runs the command 'packfilelist'
func runPackfileList(ctx context.Context, opts PackfileListOptions, gopts GlobalOptions, args []string) error {
	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock)
	if err != nil {
		return err
	}
	defer unlock()

	selectedTrees := make([]restic.ID, 0, 20)
	snapshotLister, err := restic.MemorizeList(ctx, repo, restic.SnapshotFile)
	if err != nil {
		return err
	}

	// index needs to be loaded
	if err = repo.LoadIndex(ctx, newIndexProgress(gopts.Quiet, gopts.JSON)); err != nil {
		return err
	}
	if len(args) == 0 && opts.SnapshotFilter.Empty() {
		return errors.New("No snapshots given")
	}

	// find all snapshots
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
		return errors.New("snapshotfilter active but no snapshot selected")
	}

	// gather active packfiles list
	packlist, err := CheckWithSnapshots(ctx, repo, selectedTrees, gopts)
	if err != nil {
		return err
	}

	repoPacks, err := pack.Size(ctx, repo, false)
	if err != nil {
		return err
	}
	repositorySize := int64(0)
	packfilesCount := 0
	for _, size := range repoPacks {
		repositorySize += size
		packfilesCount++
	}

	// sort
	packfilesSort := make([]restic.ID, len(packlist))
	i := 0
	for packfileID := range packlist {
		packfilesSort[i] = packfileID
		i++
	}
	slices.SortStableFunc(packfilesSort, func(a, b restic.ID) int {
		return bytes.Compare(a[:], b[:])
	})

	// output section
	if gopts.JSON {
		return produceJSONOutput(packfilesSort, packlist, repositorySize, packfilesCount)
	}

	snapSize := int64(0)
	snapSizeUsed := int64(0)
	typeCount := make(map[string]int, 2)
	for _, packfileID := range packfilesSort {
		d := packlist[packfileID]
		printID := packfileID.String()
		if opts.shortID {
			printID = printID[:8]
		}
		switch opts.detail {
		case 0:
			Printf("%s\n", printID)
		case 1:
			Printf("%s %s %10d\n", printID, d.Type, d.Size)
		case 2:
			Printf("%s %s %10d  %5d of %5d\n", printID, d.Type, d.Size, d.CountUsedBlobs, d.CountAllBlobs)
		case 3:
			Printf("%s %s %10d  %5d of %5d %10d\n", printID, d.Type, d.Size,
				d.CountUsedBlobs, d.CountAllBlobs, d.SizeUsed)
		}
		snapSize += d.Size
		snapSizeUsed += int64(d.SizeUsed)
		typeCount[d.Type]++
	}

	// summary
	Println()
	Printf("tree packfiles for snap %8d\n", typeCount["tree"])
	Printf("data packfiles for snap %8d\n", typeCount["data"])
	Printf("used size snapshots %12s\n", ui.FormatBytes(uint64(snapSizeUsed)))
	Printf("size sel snapshots  %12s\n", ui.FormatBytes(uint64(snapSize)))
	Printf("count of all packfiles  %8d\n", packfilesCount)
	Printf("size all packfiles  %12s\n", ui.FormatBytes(uint64(repositorySize)))

	return nil
}

// produceJSONOutput generates JSON output by marshalling 'packfileList'
func produceJSONOutput(packfilesSort []restic.ID, packlist map[restic.ID]PacklistInfo,
	repositorySize int64, packfilesCount int) error {

	// result JSON struct: all packfile info plus a summary
	typeCount := make(map[string]int, 2)
	var output outputStruct

	snapSize := int64(0)
	snapSizeUsed := int64(0)
	output.Summary.SizeRepo = repositorySize
	output.Summary.CountPackfiles = packfilesCount
	output.PackfileList = make([]PacklistInfo, len(packfilesSort))

	for i, packfileID := range packfilesSort {
		d := packlist[packfileID]
		output.PackfileList[i] = d
		typeCount[d.Type]++
		snapSize += d.Size
		snapSizeUsed += int64(d.SizeUsed)
	}
	output.Summary.CountTreeFiles = typeCount["tree"]
	output.Summary.CountDataFiles = typeCount["data"]
	output.Summary.SizeSnapshot = snapSize
	output.Summary.SizeSnapshotUsed = snapSizeUsed

	buf, err := json.Marshal(output)
	if err != nil {
		return err
	}
	Println(string(buf))

	return nil
}
