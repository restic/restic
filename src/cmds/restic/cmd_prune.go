package main

import (
	"fmt"
	"os"
	"restic"
	"restic/backend"
	"restic/debug"
	"restic/index"
	"restic/pack"
	"restic/repository"
	"time"

	"github.com/pkg/errors"

	"golang.org/x/crypto/ssh/terminal"
)

// CmdPrune implements the 'prune' command.
type CmdPrune struct {
	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("prune",
		"removes content from a repository",
		`
The prune command removes rendundant and unneeded data from the repository.
For removing snapshots, please see the 'forget' command, then afterwards run
'prune'.
`,
		&CmdPrune{global: &globalOpts})
	if err != nil {
		panic(err)
	}
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

// Execute runs the 'prune' command.
func (cmd CmdPrune) Execute(args []string) error {
	repo, err := cmd.global.OpenRepository()
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

	cmd.global.Verbosef("counting files in repo\n")
	for _ = range repo.List(backend.Data, done) {
		stats.packs++
	}

	cmd.global.Verbosef("building new index for repo\n")

	bar := newProgressMax(cmd.global.ShowProgress(), uint64(stats.packs), "packs")
	idx, err := index.New(repo, bar)
	if err != nil {
		return err
	}

	for _, pack := range idx.Packs {
		stats.bytes += pack.Size
	}
	cmd.global.Verbosef("repository contains %v packs (%v blobs) with %v bytes\n",
		len(idx.Packs), len(idx.Blobs), formatBytes(uint64(stats.bytes)))

	blobCount := make(map[pack.Handle]int)
	duplicateBlobs := 0
	duplicateBytes := 0

	// find duplicate blobs
	for _, p := range idx.Packs {
		for _, entry := range p.Entries {
			stats.blobs++
			h := pack.Handle{ID: entry.ID, Type: entry.Type}
			blobCount[h]++

			if blobCount[h] > 1 {
				duplicateBlobs++
				duplicateBytes += int(entry.Length)
			}
		}
	}

	cmd.global.Verbosef("processed %d blobs: %d duplicate blobs, %v duplicate\n",
		stats.blobs, duplicateBlobs, formatBytes(uint64(duplicateBytes)))
	cmd.global.Verbosef("load all snapshots\n")

	// find referenced blobs
	snapshots, err := restic.LoadAllSnapshots(repo)
	if err != nil {
		return err
	}

	stats.snapshots = len(snapshots)

	cmd.global.Verbosef("find data that is still in use for %d snapshots\n", stats.snapshots)

	usedBlobs := pack.NewBlobSet()
	seenBlobs := pack.NewBlobSet()

	bar = newProgressMax(cmd.global.ShowProgress(), uint64(len(snapshots)), "snapshots")
	bar.Start()
	for _, sn := range snapshots {
		debug.Log("CmdPrune.Execute", "process snapshot %v", sn.ID().Str())

		err = restic.FindUsedBlobs(repo, *sn.Tree, usedBlobs, seenBlobs)
		if err != nil {
			return err
		}

		debug.Log("CmdPrune.Execute", "found %v blobs for snapshot %v", sn.ID().Str())
		bar.Report(restic.Stat{Blobs: 1})
	}
	bar.Done()

	cmd.global.Verbosef("found %d of %d data blobs still in use\n", len(usedBlobs), stats.blobs)

	// find packs that need a rewrite
	rewritePacks := backend.NewIDSet()
	for h, blob := range idx.Blobs {
		if !usedBlobs.Has(h) {
			rewritePacks.Merge(blob.Packs)
			continue
		}

		if blobCount[h] > 1 {
			rewritePacks.Merge(blob.Packs)
		}
	}

	// find packs that are unneeded
	removePacks := backend.NewIDSet()
nextPack:
	for packID, p := range idx.Packs {
		for _, blob := range p.Entries {
			h := pack.Handle{ID: blob.ID, Type: blob.Type}
			if usedBlobs.Has(h) {
				continue nextPack
			}
		}

		removePacks.Insert(packID)

		if !rewritePacks.Has(packID) {
			return errors.Errorf("pack %v is unneeded, but not contained in rewritePacks", packID.Str())
		}

		rewritePacks.Delete(packID)
	}

	cmd.global.Verbosef("will delete %d packs and rewrite %d packs\n", len(removePacks), len(rewritePacks))

	err = repository.Repack(repo, rewritePacks, usedBlobs)
	if err != nil {
		return err
	}

	for packID := range removePacks {
		err = repo.Backend().Remove(backend.Data, packID.String())
		if err != nil {
			cmd.global.Warnf("unable to remove file %v from the repository\n", packID.Str())
		}
	}

	cmd.global.Verbosef("creating new index\n")

	stats.packs = 0
	for _ = range repo.List(backend.Data, done) {
		stats.packs++
	}
	bar = newProgressMax(cmd.global.ShowProgress(), uint64(stats.packs), "packs")
	idx, err = index.New(repo, bar)
	if err != nil {
		return err
	}

	var supersedes backend.IDs
	for idxID := range repo.List(backend.Index, done) {
		err := repo.Backend().Remove(backend.Index, idxID.String())
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to remove index %v: %v\n", idxID.Str(), err)
		}

		supersedes = append(supersedes, idxID)
	}

	id, err := idx.Save(repo, supersedes)
	if err != nil {
		return err
	}
	cmd.global.Verbosef("saved new index as %v\n", id.Str())

	cmd.global.Verbosef("done\n")
	return nil
}
