package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"slices"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
)

// PacklistInfo defines one entry per packfile
type PacklistInfo struct {
	ID   restic.ID `json:"id"`
	Type string    `json:"type"`
	Size int64     `json:"size"`
}

type PFInfo struct {
	snapPacks map[restic.ID]PacklistInfo
	printer   progress.Printer
}

// output definition for JSON
type outputStruct struct {
	PackfileList []PacklistInfo `json:"packfiles"`
}

// CheckWithSnapshots will process snapshot IDs from 'selectedTrees'
func (pfInfo *PFInfo) CheckWithSnapshots(ctx context.Context, repo *repository.Repository, selectedTrees []restic.ID) error {

	// gather used blobs from all trees in 'selectedTrees'
	usedBlobs := restic.NewBlobSet()
	if err := data.FindUsedBlobs(ctx, repo, selectedTrees, usedBlobs, nil); err != nil {
		return err
	}

	// get length of packfiles from repository via index
	repoPacks, err := pack.Size(ctx, repo, false)
	if err != nil {
		return err
	}

	// convert used blobs to packfile IDs and collect statistics
	for blobHandle := range usedBlobs {
		for _, blob := range repo.LookupBlob(blobHandle.Type, blobHandle.ID) {
			if _, ok := pfInfo.snapPacks[blob.PackID]; !ok {
				pfInfo.snapPacks[blob.PackID] = PacklistInfo{
					ID:   blob.PackID,
					Type: blob.Type.String(),
					Size: repoPacks[blob.PackID],
				}
			}
		}
	}

	return nil
}

// runPackfileList runs the command 'packfilelist'
func runPackfileList(ctx context.Context, opts RestoreOptions, gopts GlobalOptions, args []string, printer progress.Printer) error {
	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, true, printer)
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
	if err = repo.LoadIndex(ctx, printer); err != nil {
		return err
	}

	// find all selected snapshots
	err = (&opts.SnapshotFilter).FindAll(ctx, snapshotLister, repo, args, func(_ string, sn *data.Snapshot, err error) error {
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

	// gather packfiles list
	pfInfo := &PFInfo{
		snapPacks: make(map[restic.ID]PacklistInfo),
		printer:   printer,
	}
	if err = pfInfo.CheckWithSnapshots(ctx, repo, selectedTrees); err != nil {
		return err
	}

	// sort packfile IDs
	packfilesSort := make([]restic.ID, 0, len(pfInfo.snapPacks))
	for packfileID := range pfInfo.snapPacks {
		packfilesSort = append(packfilesSort, packfileID)
	}
	slices.SortStableFunc(packfilesSort, func(a, b restic.ID) int {
		return bytes.Compare(a[:], b[:])
	})

	if gopts.JSON {
		return produceJSONOutput(packfilesSort, pfInfo.snapPacks, gopts.term.OutputWriter())
	}

	pfInfo.produceTextOutput(packfilesSort, gopts, printer)
	return nil
}

// produceJSONOutput generates JSON output
func produceJSONOutput(packfilesSort []restic.ID, snapPacks map[restic.ID]PacklistInfo, stdout io.Writer) error {
	var output outputStruct
	output.PackfileList = make([]PacklistInfo, len(packfilesSort))
	for i, packfileID := range packfilesSort {
		output.PackfileList[i] = snapPacks[packfileID]
	}

	return json.NewEncoder(stdout).Encode(output)
}

func (pfInfo *PFInfo) produceTextOutput(packfilesSort []restic.ID, gopts GlobalOptions, printer progress.Printer) {
	for _, packfileID := range packfilesSort {
		d := pfInfo.snapPacks[packfileID]

		if gopts.Verbose == 0 {
			printer.P("%s", packfileID.String())
		} else {
			printer.P("%s %s %10d", packfileID.String(), d.Type, d.Size)
		}
	}
}
