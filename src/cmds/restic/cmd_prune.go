package main

import (
	"fmt"
	"os"
	"restic"
	"restic/backend"
	"restic/debug"
	"restic/list"
	"restic/pack"
	"restic/repository"
	"restic/worker"
	"time"

	"golang.org/x/crypto/ssh/terminal"
)

// CmdPrune implements the 'prune' command.
type CmdPrune struct {
	global *GlobalOptions
}

func init() {
	_, err := parser.AddCommand("prune",
		"removes content from a repository",
		"The prune command removes rendundant and unneeded data from the repository",
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

	p := restic.NewProgress(time.Second)

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

		fmt.Printf("\x1b[2K%s\r", status)
	}

	p.OnDone = func(s restic.Stat, d time.Duration, ticker bool) {
		p.OnUpdate(s, d, false)
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

	cmd.global.Verbosef("loading list of files from the repo\n")

	var stats struct {
		blobs     int
		packs     int
		snapshots int
	}

	packs := make(map[backend.ID]pack.BlobSet)
	for packID := range repo.List(backend.Data, done) {
		debug.Log("CmdPrune.Execute", "found %v", packID.Str())
		packs[packID] = pack.NewBlobSet()
		stats.packs++
	}

	cmd.global.Verbosef("listing %v files\n", stats.packs)

	blobCount := make(map[backend.ID]int)
	duplicateBlobs := 0
	duplicateBytes := 0
	rewritePacks := backend.NewIDSet()

	ch := make(chan worker.Job)
	go list.AllPacks(repo, ch, done)

	bar := newProgressMax(cmd.global.ShowProgress(), uint64(len(packs)), "files")
	bar.Start()
	for job := range ch {
		packID := job.Data.(backend.ID)
		if job.Error != nil {
			cmd.global.Warnf("unable to list pack %v: %v\n", packID.Str(), job.Error)
			continue
		}

		j := job.Result.(list.Result)

		debug.Log("CmdPrune.Execute", "pack %v contains %d blobs", packID.Str(), len(j.Entries()))
		for _, pb := range j.Entries() {
			packs[packID].Insert(pack.Handle{ID: pb.ID, Type: pb.Type})
			stats.blobs++
			blobCount[pb.ID]++

			if blobCount[pb.ID] > 1 {
				duplicateBlobs++
				duplicateBytes += int(pb.Length)
			}
		}
		bar.Report(restic.Stat{Blobs: 1})
	}
	bar.Done()

	cmd.global.Verbosef("processed %d blobs: %d duplicate blobs, %d duplicate bytes\n",
		stats.blobs, duplicateBlobs, duplicateBytes)
	cmd.global.Verbosef("load all snapshots\n")

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

	for packID, blobSet := range packs {
		for h := range blobSet {
			if !usedBlobs.Has(h) {
				rewritePacks.Insert(packID)
			}

			if blobCount[h.ID] > 1 {
				rewritePacks.Insert(packID)
			}
		}
	}

	cmd.global.Verbosef("will rewrite %d packs\n", len(rewritePacks))

	err = repository.Repack(repo, rewritePacks, usedBlobs)
	if err != nil {
		return err
	}

	cmd.global.Verbosef("creating new index\n")

	err = repository.RebuildIndex(repo)
	if err != nil {
		return err
	}

	cmd.global.Verbosef("done\n")
	return nil
}
