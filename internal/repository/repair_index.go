package repository

import (
	"context"

	"github.com/restic/restic/internal/repository/index"
	"github.com/restic/restic/internal/repository/pack"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
)

type RepairIndexOptions struct {
	ReadAllPacks bool
}

func RepairIndex(ctx context.Context, repo *Repository, opts RepairIndexOptions, printer progress.Printer) error {
	var obsoleteIndexes restic.IDs
	packSizeFromList := make(map[restic.ID]int64)
	packSizeFromIndex := make(map[restic.ID]int64)
	removePacks := restic.NewIDSet()

	if opts.ReadAllPacks {
		// get list of old index files but start with empty index
		err := repo.List(ctx, restic.IndexFile, func(id restic.ID, _ int64) error {
			obsoleteIndexes = append(obsoleteIndexes, id)
			return nil
		})
		if err != nil {
			return err
		}
		repo.clearIndex()

	} else {
		printer.P("loading indexes...\n")
		mi := index.NewMasterIndex()
		err := index.ForAllIndexes(ctx, repo, repo, func(id restic.ID, idx *index.Index, _ bool, err error) error {
			if err != nil {
				printer.E("removing invalid index %v: %v\n", id, err)
				obsoleteIndexes = append(obsoleteIndexes, id)
				return nil
			}

			mi.Insert(idx)
			return nil
		})
		if err != nil {
			return err
		}

		err = mi.MergeFinalIndexes()
		if err != nil {
			return err
		}

		err = repo.SetIndex(mi)
		if err != nil {
			return err
		}
		packSizeFromIndex, err = pack.Size(ctx, repo, false)
		if err != nil {
			return err
		}
	}

	oldIndexes := repo.idx.IDs()

	printer.P("getting pack files to read...\n")
	err := repo.List(ctx, restic.PackFile, func(id restic.ID, packSize int64) error {
		size, ok := packSizeFromIndex[id]
		if !ok || size != packSize {
			// Pack was not referenced in index or size does not match
			packSizeFromList[id] = packSize
			removePacks.Insert(id)
		}
		if !ok {
			printer.E("adding pack file to index %v\n", id)
		} else if size != packSize {
			printer.E("reindexing pack file %v with unexpected size %v instead of %v\n", id, packSize, size)
		}
		delete(packSizeFromIndex, id)
		return nil
	})
	if err != nil {
		return err
	}
	for id := range packSizeFromIndex {
		// forget pack files that are referenced in the index but do not exist
		// when rebuilding the index
		removePacks.Insert(id)
		printer.E("removing not found pack file %v\n", id)
	}

	if len(packSizeFromList) > 0 {
		printer.P("reading pack files\n")
		bar := printer.NewCounter("packs")
		bar.SetMax(uint64(len(packSizeFromList)))
		invalidFiles, err := repo.createIndexFromPacks(ctx, packSizeFromList, bar)
		bar.Done()
		if err != nil {
			return err
		}

		for _, id := range invalidFiles {
			printer.V("skipped incomplete pack file: %v\n", id)
		}
	}

	if err := repo.Flush(ctx); err != nil {
		return err
	}

	err = rewriteIndexFiles(ctx, repo, removePacks, oldIndexes, obsoleteIndexes, printer)
	if err != nil {
		return err
	}

	// drop outdated in-memory index
	repo.clearIndex()
	return nil
}

func rewriteIndexFiles(ctx context.Context, repo *Repository, removePacks restic.IDSet, oldIndexes restic.IDSet, extraObsolete restic.IDs, printer progress.Printer) error {
	printer.P("rebuilding index\n")

	bar := printer.NewCounter("indexes processed")
	return repo.idx.Rewrite(ctx, repo, removePacks, oldIndexes, extraObsolete, index.MasterIndexRewriteOpts{
		SaveProgress: bar,
		DeleteProgress: func() *progress.Counter {
			return printer.NewCounter("old indexes deleted")
		},
		DeleteReport: func(id restic.ID, err error) {
			if err != nil {
				printer.VV("failed to remove index %v: %v\n", id.String(), err)
			} else {
				printer.VV("removed index %v\n", id.String())
			}
		},
	})
}
