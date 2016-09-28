package main

import (
	"fmt"
	"os"
	"restic"
	"restic/debug"
	"restic/errors"
	"restic/index"
	"restic/repository"
	"time"

	"github.com/spf13/cobra"

	"golang.org/x/crypto/ssh/terminal"
)

var cmdPrune = &cobra.Command{
	Use:   "prune [flags]",
	Short: "remove unneeded data from the repository",
	Long: `
The "prune" command checks the repository and removes data that is not
referenced and therefore not needed any more.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPrune(globalOptions)
	},
}

func init() {
	cmdRoot.AddCommand(cmdPrune)
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

		w, _, err := terminal.GetSize(int(os.Stdout.Fd()))
		if err == nil {
			if len(status) > w {
				max := w - len(status) - 4
				status = status[:max] + "... "
			}
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

	err = repo.LoadIndex()
	if err != nil {
		return err
	}

	done := make(chan struct{})
	defer close(done)

	var stats struct {
		blobs     int
		packs     int
		snapshots int
		bytes     int64
	}

	Verbosef("counting files in repo\n")
	for _ = range repo.List(restic.DataFile, done) {
		stats.packs++
	}

	Verbosef("building new index for repo\n")

	bar := newProgressMax(!gopts.Quiet, uint64(stats.packs), "packs")
	idx, err := index.New(repo, bar)
	if err != nil {
		return err
	}

	for _, pack := range idx.Packs {
		stats.bytes += pack.Size
	}
	Verbosef("repository contains %v packs (%v blobs) with %v bytes\n",
		len(idx.Packs), len(idx.Blobs), formatBytes(uint64(stats.bytes)))

	blobCount := make(map[restic.BlobHandle]int)
	duplicateBlobs := 0
	duplicateBytes := 0

	// find duplicate blobs
	for _, p := range idx.Packs {
		for _, entry := range p.Entries {
			stats.blobs++
			h := restic.BlobHandle{ID: entry.ID, Type: entry.Type}
			blobCount[h]++

			if blobCount[h] > 1 {
				duplicateBlobs++
				duplicateBytes += int(entry.Length)
			}
		}
	}

	Verbosef("processed %d blobs: %d duplicate blobs, %v duplicate\n",
		stats.blobs, duplicateBlobs, formatBytes(uint64(duplicateBytes)))
	Verbosef("load all snapshots\n")

	// find referenced blobs
	snapshots, err := restic.LoadAllSnapshots(repo)
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
		debug.Log("process snapshot %v", sn.ID().Str())

		err = restic.FindUsedBlobs(repo, *sn.Tree, usedBlobs, seenBlobs)
		if err != nil {
			return err
		}

		debug.Log("found %v blobs for snapshot %v", sn.ID().Str())
		bar.Report(restic.Stat{Blobs: 1})
	}
	bar.Done()

	Verbosef("found %d of %d data blobs still in use, removing %d blobs\n",
		len(usedBlobs), stats.blobs, stats.blobs-len(usedBlobs))

	// find packs that need a rewrite
	rewritePacks := restic.NewIDSet()
	for h, blob := range idx.Blobs {
		if !usedBlobs.Has(h) {
			rewritePacks.Merge(blob.Packs)
			continue
		}

		if blobCount[h] > 1 {
			rewritePacks.Merge(blob.Packs)
		}
	}

	removeBytes := 0

	// find packs that are unneeded
	removePacks := restic.NewIDSet()
	for packID, p := range idx.Packs {

		hasActiveBlob := false
		for _, blob := range p.Entries {
			h := restic.BlobHandle{ID: blob.ID, Type: blob.Type}
			if usedBlobs.Has(h) {
				hasActiveBlob = true
				continue
			}

			removeBytes += int(blob.Length)
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

	err = repository.Repack(repo, rewritePacks, usedBlobs)
	if err != nil {
		return err
	}

	for packID := range removePacks {
		err = repo.Backend().Remove(restic.DataFile, packID.String())
		if err != nil {
			Warnf("unable to remove file %v from the repository\n", packID.Str())
		}
	}

	Verbosef("creating new index\n")

	stats.packs = 0
	for _ = range repo.List(restic.DataFile, done) {
		stats.packs++
	}
	bar = newProgressMax(!gopts.Quiet, uint64(stats.packs), "packs")
	idx, err = index.New(repo, bar)
	if err != nil {
		return err
	}

	var supersedes restic.IDs
	for idxID := range repo.List(restic.IndexFile, done) {
		err := repo.Backend().Remove(restic.IndexFile, idxID.String())
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to remove index %v: %v\n", idxID.Str(), err)
		}

		supersedes = append(supersedes, idxID)
	}

	id, err := idx.Save(repo, supersedes)
	if err != nil {
		return err
	}
	Verbosef("saved new index as %v\n", id.Str())

	Verbosef("done\n")
	return nil
}
