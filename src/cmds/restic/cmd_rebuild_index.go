package main

import (
	"fmt"
	"os"
	"restic/backend"
	"restic/debug"
	"restic/pack"
	"restic/repository"
	"restic/worker"
)

type CmdRebuildIndex struct {
	global *GlobalOptions

	repo *repository.Repository
}

func init() {
	_, err := parser.AddCommand("rebuild-index",
		"rebuild the index",
		"The rebuild-index command builds a new index",
		&CmdRebuildIndex{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

const rebuildIndexWorkers = 10

func loadBlobsFromPacks(repo *repository.Repository) (packs map[backend.ID][]pack.Blob) {
	done := make(chan struct{})
	defer close(done)

	f := func(job worker.Job, done <-chan struct{}) (interface{}, error) {
		return repo.ListPack(job.Data.(backend.ID))
	}

	jobCh := make(chan worker.Job)
	resCh := make(chan worker.Job)
	wp := worker.New(rebuildIndexWorkers, f, jobCh, resCh)

	go func() {
		for id := range repo.List(backend.Data, done) {
			jobCh <- worker.Job{Data: id}
		}
		close(jobCh)
	}()

	packs = make(map[backend.ID][]pack.Blob)
	for job := range resCh {
		id := job.Data.(backend.ID)

		if job.Error != nil {
			fmt.Fprintf(os.Stderr, "error for pack %v: %v\n", id, job.Error)
			continue
		}

		entries := job.Result.([]pack.Blob)
		packs[id] = entries
	}

	wp.Wait()

	return packs
}

func listIndexIDs(repo *repository.Repository) (list backend.IDs) {
	done := make(chan struct{})
	for id := range repo.List(backend.Index, done) {
		list = append(list, id)
	}

	return list
}

func (cmd CmdRebuildIndex) rebuildIndex() error {
	debug.Log("RebuildIndex.RebuildIndex", "start rebuilding index")

	packs := loadBlobsFromPacks(cmd.repo)
	cmd.global.Verbosef("loaded blobs from %d packs\n", len(packs))

	idx := repository.NewIndex()
	for packID, entries := range packs {
		for _, entry := range entries {
			pb := repository.PackedBlob{
				ID:     entry.ID,
				Type:   entry.Type,
				Length: entry.Length,
				Offset: entry.Offset,
				PackID: packID,
			}
			idx.Store(pb)
		}
	}

	oldIndexes := listIndexIDs(cmd.repo)
	idx.AddToSupersedes(oldIndexes...)
	cmd.global.Printf("  saving new index\n")
	id, err := repository.SaveIndex(cmd.repo, idx)
	if err != nil {
		debug.Log("RebuildIndex.RebuildIndex", "error saving index: %v", err)
		return err
	}
	debug.Log("RebuildIndex.RebuildIndex", "new index saved as %v", id.Str())

	for _, indexID := range oldIndexes {
		err := cmd.repo.Backend().Remove(backend.Index, indexID.String())
		if err != nil {
			cmd.global.Warnf("unable to remove index %v: %v\n", indexID.Str(), err)
		}
	}

	return nil
}

func (cmd CmdRebuildIndex) Execute(args []string) error {
	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}
	cmd.repo = repo

	lock, err := lockRepoExclusive(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	return cmd.rebuildIndex()
}
