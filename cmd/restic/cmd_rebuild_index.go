package main

import (
	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/repository"
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

func (cmd CmdRebuildIndex) RebuildIndex() error {
	debug.Log("RebuildIndex.RebuildIndex", "start")

	done := make(chan struct{})
	defer close(done)

	indexIDs := backend.NewIDSet()
	for id := range cmd.repo.List(backend.Index, done) {
		indexIDs.Insert(id)
	}

	debug.Log("RebuildIndex.RebuildIndex", "found %v indexes", len(indexIDs))

	var combinedIndex *repository.Index

	for indexID := range indexIDs {
		debug.Log("RebuildIndex.RebuildIndex", "load index %v", indexID.Str())
		idx, err := repository.LoadIndex(cmd.repo, indexID.String())
		if err != nil {
			return err
		}

		debug.Log("RebuildIndex.RebuildIndex", "adding blobs from index %v", indexID.Str())

		if combinedIndex == nil {
			combinedIndex = repository.NewIndex()
		}

		for packedBlob := range idx.Each(done) {
			combinedIndex.Store(packedBlob.Type, packedBlob.ID, packedBlob.PackID, packedBlob.Offset, packedBlob.Length)
		}

		combinedIndex.AddToSupersedes(indexID)

		if repository.IndexFull(combinedIndex) {
			debug.Log("RebuildIndex.RebuildIndex", "saving full index")

			id, err := repository.SaveIndex(cmd.repo, combinedIndex)
			if err != nil {
				debug.Log("RebuildIndex.RebuildIndex", "error saving index: %v", err)
				return err
			}

			debug.Log("RebuildIndex.RebuildIndex", "index saved as %v", id.Str())
			combinedIndex = nil
		}
	}

	id, err := repository.SaveIndex(cmd.repo, combinedIndex)
	if err != nil {
		debug.Log("RebuildIndex.RebuildIndex", "error saving index: %v", err)
		return err
	}

	debug.Log("RebuildIndex.RebuildIndex", "last index saved as %v", id.Str())

	for id := range indexIDs {
		debug.Log("RebuildIndex.RebuildIndex", "remove index %v", id.Str())

		err = cmd.repo.Backend().Remove(backend.Index, id.String())
		if err != nil {
			debug.Log("RebuildIndex.RebuildIndex", "error removing index %v: %v", id.Str(), err)
			return err
		}
	}

	debug.Log("RebuildIndex.RebuildIndex", "done")
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

	return cmd.RebuildIndex()
}
