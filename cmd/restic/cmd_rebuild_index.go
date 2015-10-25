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

func (cmd CmdRebuildIndex) storeIndex(index *repository.Index) (*repository.Index, error) {
	debug.Log("RebuildIndex.RebuildIndex", "saving index")

	cmd.global.Printf("  saving new index\n")
	id, err := repository.SaveIndex(cmd.repo, index)
	if err != nil {
		debug.Log("RebuildIndex.RebuildIndex", "error saving index: %v", err)
		return nil, err
	}

	debug.Log("RebuildIndex.RebuildIndex", "index saved as %v", id.Str())
	index = repository.NewIndex()

	return index, nil
}

func (cmd CmdRebuildIndex) RebuildIndex() error {
	debug.Log("RebuildIndex.RebuildIndex", "start")

	done := make(chan struct{})
	defer close(done)

	indexIDs := backend.NewIDSet()
	for id := range cmd.repo.List(backend.Index, done) {
		indexIDs.Insert(id)
	}

	cmd.global.Printf("rebuilding index from %d indexes\n", len(indexIDs))

	debug.Log("RebuildIndex.RebuildIndex", "found %v indexes", len(indexIDs))

	combinedIndex := repository.NewIndex()

	i := 0
	for indexID := range indexIDs {
		cmd.global.Printf("  loading index %v\n", i)

		debug.Log("RebuildIndex.RebuildIndex", "load index %v", indexID.Str())
		idx, err := repository.LoadIndex(cmd.repo, indexID.String())
		if err != nil {
			return err
		}

		debug.Log("RebuildIndex.RebuildIndex", "adding blobs from index %v", indexID.Str())

		for packedBlob := range idx.Each(done) {
			combinedIndex.Store(packedBlob.Type, packedBlob.ID, packedBlob.PackID, packedBlob.Offset, packedBlob.Length)
		}

		combinedIndex.AddToSupersedes(indexID)

		if repository.IndexFull(combinedIndex) {
			combinedIndex, err = cmd.storeIndex(combinedIndex)
			if err != nil {
				return err
			}
		}

		i++
	}

	var err error
	if combinedIndex.Length() > 0 {
		combinedIndex, err = cmd.storeIndex(combinedIndex)
		if err != nil {
			return err
		}
	}

	cmd.global.Printf("removing %d old indexes\n", len(indexIDs))
	for id := range indexIDs {
		debug.Log("RebuildIndex.RebuildIndex", "remove index %v", id.Str())

		err := cmd.repo.Backend().Remove(backend.Index, id.String())
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
