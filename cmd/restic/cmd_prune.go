package main

import (
	"fmt"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/index"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
)

var cmdPrune = &cobra.Command{
	Use:   "prune [flags]",
	Short: "Remove unneeded data from the repository",
	Long: `
The "prune" command checks the repository and removes data that is not
referenced and therefore not needed any more.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPrune(globalOptions)
	},
}

func init() {
	cmdRoot.AddCommand(cmdPrune)
}

func shortenStatus(maxLength int, s string) string {
	if len(s) <= maxLength {
		return s
	}

	if maxLength < 3 {
		return s[:maxLength]
	}

	return s[:maxLength-3] + "..."
}

// newProgressMax returns a progress that counts blobs.
func newProgressMax(show bool, max uint64, description string) *restic.Progress {
	if !show {
		return nil
	}

	p := restic.NewProgress()

	p.OnUpdate = func(s restic.Stat, d time.Duration, ticker bool) {
		status := fmt.Sprintf("[%s] %s  %d / %d %s",
			formatDuration(d),
			formatPercent(s.Blobs, max),
			s.Blobs, max, description)

		if w := stdoutTerminalWidth(); w > 0 {
			status = shortenStatus(w, status)
		}

		PrintProgress("%s", status)
	}

	p.OnDone = func(s restic.Stat, d time.Duration, ticker bool) {
		fmt.Printf("\n")
	}

	return p
}

func runPrune(gopts GlobalOptions) error {
	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	lock, err := lockRepoExclusive(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	return pruneRepository(gopts, repo)
}

func mixedBlobs(list []restic.Blob) bool {
	var tree, data bool

	for _, pb := range list {
		switch pb.Type {
		case restic.TreeBlob:
			tree = true
		case restic.DataBlob:
			data = true
		}

		if tree && data {
			return true
		}
	}

	return false
}

func pruneRepository(gopts GlobalOptions, repo restic.Repository) error {
	ctx := gopts.ctx

	err := repo.LoadIndex(ctx)
	if err != nil {
		return err
	}

	var stats struct {
		blobs     int
		packs     int
		snapshots int
		bytes     int64
	}

	Verbosef("counting files in repo\n")
	err = repo.List(ctx, restic.DataFile, func(restic.ID, int64) error {
		stats.packs++
		return nil
	})
	if err != nil {
		return err
	}

	Verbosef("building new index for repo\n")

	bar := newProgressMax(!gopts.Quiet, uint64(stats.packs), "packs")
	idx, invalidFiles, err := index.New(ctx, repo, restic.NewIDSet(), bar)
	if err != nil {
		return err
	}

	for _, id := range invalidFiles {
		Warnf("incomplete pack file (will be removed): %v\n", id)
	}

	blobs := 0
	for _, pack := range idx.Packs {
		stats.bytes += pack.Size
		blobs += len(pack.Entries)
	}
	Verbosef("repository contains %v packs (%v blobs) with %v\n",
		len(idx.Packs), blobs, formatBytes(uint64(stats.bytes)))

	blobCount := make(map[restic.BlobHandle]int)
	var duplicateBlobs uint64
	var duplicateBytes uint64

	// find duplicate blobs
	for _, p := range idx.Packs {
		for _, entry := range p.Entries {
			stats.blobs++
			h := restic.BlobHandle{ID: entry.ID, Type: entry.Type}
			blobCount[h]++

			if blobCount[h] > 1 {
				duplicateBlobs++
				duplicateBytes += uint64(entry.Length)
			}
		}
	}

	Verbosef("processed %d blobs: %d duplicate blobs, %v duplicate\n",
		stats.blobs, duplicateBlobs, formatBytes(uint64(duplicateBytes)))
	Verbosef("load all snapshots\n")

	// find referenced blobs
	snapshots, err := restic.LoadAllSnapshots(ctx, repo)
	if err != nil {
		return err
	}

	stats.snapshots = len(snapshots)

	Verbosef("find data that is still in use for %d snapshots\n", stats.snapshots)

	usedBlobs := restic.NewBlobSet()
	seenBlobs := restic.NewBlobSet()

	bar = newProgressMax(!gopts.Quiet, uint64(len(snapshots)), "snapshots")
	bar.Start()
	for _, sn := range snapshots {
		debug.Log("process snapshot %v", sn.ID())

		err = restic.FindUsedBlobs(ctx, repo, *sn.Tree, usedBlobs, seenBlobs)
		if err != nil {
			if repo.Backend().IsNotExist(err) {
				return errors.Fatal("unable to load a tree from the repo: " + err.Error())
			}

			return err
		}

		debug.Log("processed snapshot %v", sn.ID())
		bar.Report(restic.Stat{Blobs: 1})
	}
	bar.Done()

	if len(usedBlobs) > stats.blobs {
		return errors.Fatalf("number of used blobs is larger than number of available blobs!\n" +
			"Please report this error (along with the output of the 'prune' run) at\n" +
			"https://github.com/restic/restic/issues/new")
	}

	Verbosef("found %d of %d data blobs still in use, removing %d blobs\n",
		len(usedBlobs), stats.blobs, stats.blobs-len(usedBlobs))

	// find packs that need a rewrite
	rewritePacks := restic.NewIDSet()
	for _, pack := range idx.Packs {
		if mixedBlobs(pack.Entries) {
			rewritePacks.Insert(pack.ID)
			continue
		}

		for _, blob := range pack.Entries {
			h := restic.BlobHandle{ID: blob.ID, Type: blob.Type}
			if !usedBlobs.Has(h) {
				rewritePacks.Insert(pack.ID)
				continue
			}

			if blobCount[h] > 1 {
				rewritePacks.Insert(pack.ID)
			}
		}
	}

	removeBytes := duplicateBytes

	// find packs that are unneeded
	removePacks := restic.NewIDSet()

	Verbosef("will remove %d invalid files\n", len(invalidFiles))
	for _, id := range invalidFiles {
		removePacks.Insert(id)
	}

	for packID, p := range idx.Packs {

		hasActiveBlob := false
		for _, blob := range p.Entries {
			h := restic.BlobHandle{ID: blob.ID, Type: blob.Type}
			if usedBlobs.Has(h) {
				hasActiveBlob = true
				continue
			}

			removeBytes += uint64(blob.Length)
		}

		if hasActiveBlob {
			continue
		}

		removePacks.Insert(packID)

		if !rewritePacks.Has(packID) {
			return errors.Fatalf("pack %v is unneeded, but not contained in rewritePacks", packID.Str())
		}

		rewritePacks.Delete(packID)
	}

	Verbosef("will delete %d packs and rewrite %d packs, this frees %s\n",
		len(removePacks), len(rewritePacks), formatBytes(uint64(removeBytes)))

	var obsoletePacks restic.IDSet
	if len(rewritePacks) != 0 {
		bar = newProgressMax(!gopts.Quiet, uint64(len(rewritePacks)), "packs rewritten")
		bar.Start()
		obsoletePacks, err = repository.Repack(ctx, repo, rewritePacks, usedBlobs, bar)
		if err != nil {
			return err
		}
		bar.Done()
	}

	removePacks.Merge(obsoletePacks)

	if err = rebuildIndex(ctx, repo, removePacks); err != nil {
		return err
	}

	if len(removePacks) != 0 {
		bar = newProgressMax(!gopts.Quiet, uint64(len(removePacks)), "packs deleted")
		bar.Start()
		for packID := range removePacks {
			h := restic.Handle{Type: restic.DataFile, Name: packID.String()}
			err = repo.Backend().Remove(ctx, h)
			if err != nil {
				Warnf("unable to remove file %v from the repository\n", packID.Str())
			}
			bar.Report(restic.Stat{Blobs: 1})
		}
		bar.Done()
	}

	Verbosef("done\n")
	return nil
}
