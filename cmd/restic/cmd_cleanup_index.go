package main

import (
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"

	"github.com/spf13/cobra"
)

var cmdCleanupIndex = &cobra.Command{
	Use:   "cleanup-index [flags]",
	Short: "Remove unused blobs from index",
	Long: `
The "cleanup-index" command removes data from the index 
that is not referenced and therefore not needed any more.`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCleanupIndex(cleanupIndexOptions, globalOptions)
	},
}

// CleanupIndexOptions collects all options for the cleanup-index command.
type CleanupIndexOptions struct {
	DryRun bool
}

var cleanupIndexOptions CleanupIndexOptions

func init() {
	cmdRoot.AddCommand(cmdCleanupIndex)

	f := cmdCleanupIndex.Flags()
	f.BoolVarP(&cleanupIndexOptions.DryRun, "dry-run", "n", false, "do not delete anything, just print what would be done")
}

func runCleanupIndex(opts CleanupIndexOptions, gopts GlobalOptions) error {
	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	lock, err := lockRepoExclusive(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	// get snapshot list
	Verbosef("get all snapshots\n")
	snapshots, err := restic.LoadAllSnapshots(gopts.ctx, repo)
	if err != nil {
		return err
	}

	// find referenced blobs
	Verbosef("load indexes\n")
	err = repo.LoadIndex(gopts.ctx)
	if err != nil {
		return err
	}

	usedBlobs, err := getUsedBlobs(gopts, repo, snapshots)
	if err != nil {
		return err
	}

	err = CleanupIndex(opts, gopts, repo, usedBlobs)
	if err != nil {
		return err
	}

	if len(usedBlobs) > 0 {
		Warnf("There are blobs in use which are not referenced in the index files:\n")
		for blob := range usedBlobs {
			Warnf("%v\n", blob)
		}
	}
	return nil
}

func CleanupIndex(opts CleanupIndexOptions, gopts GlobalOptions, repo restic.Repository, usedBlobs restic.BlobSet) error {
	return ModifyIndex(opts.DryRun, gopts, repo, func(pb restic.PackedBlob) (changed bool, pbnew restic.PackedBlob) {
		h := restic.BlobHandle{ID: pb.ID, Type: pb.Type}
		if !usedBlobs.Has(h) {
			// delete Blob from index
			changed = true
			return
		}
		// keep only once
		usedBlobs.Delete(h)
		// blob remains unchanged
		return
	})
}

// ModifyIndex modifies all index files with respect to a selector function
// TODO: This function is a work-around for missing functionality in the Index data structure
//       Index should be implemented such that it lines like the following work:
//		for pb := range repo.Index().Each(ctx) {
//			change, newpb := f(pb)
//			if change {
//				if (newpb != restic.PackedBlob{}) {
//					repo.Index().Modify(newpb)
//				} else {
//					repo.Index().Delete(pb.ID, pb.Type)
//				}
//			}
//		}
//		repo.SaveModifiedIndices()
func ModifyIndex(dryRun bool, gopts GlobalOptions, repo restic.Repository, f func(restic.PackedBlob) (bool, restic.PackedBlob)) error {
	ctx := gopts.ctx

	indexlist := restic.NewIDSet()
	// TODO: Add parallel processing
	err := repo.List(ctx, restic.IndexFile, func(id restic.ID, size int64) error {
		indexlist.Insert(id)
		return nil
	})
	if err != nil {
		return err
	}

	Verbosef("check %d index files and change if neccessary\n", len(indexlist))
	bar := newProgressMax(!gopts.Quiet, uint64(len(indexlist)), "index files processed")
	bar.Start()
	// TODO: Add parallel processing
	for id := range indexlist {
		idxNew := repository.NewIndex()
		err := idxNew.AddToSupersedes(id)
		if err != nil {
			return err
		}

		idx, err := repository.LoadIndex(ctx, repo, id)
		if err != nil {
			return err
		}

		changed := false
		for pb := range idx.Each(ctx) {
			change, newpb := f(pb)
			if change {
				if (newpb != restic.PackedBlob{}) {
					idxNew.Store(newpb)
				}
				changed = true
			} else {
				idxNew.Store(pb)
			}
		}
		if changed {
			if !dryRun {
				newID, err := repository.SaveIndex(ctx, repo, idxNew)
				if err != nil {
					return err
				}
				h := restic.Handle{Type: restic.IndexFile, Name: id.String()}
				err = repo.Backend().Remove(ctx, h)
				if err != nil {
					Warnf("unable to remove index %v from the repository\n", id.Str())
				}
				if !gopts.JSON {
					Verbosef("index %v was removed. new index: %v\n", id.Str(), newID.Str())
				}
			} else {
				if !gopts.JSON {
					Verbosef("would have replaced index %v\n", id.Str())
				}
			}
		}
		bar.Report(restic.Stat{Files: 1})
	}
	bar.Done()

	Verbosef("done\n")
	return nil
}

func getUsedBlobs(gopts GlobalOptions, repo restic.Repository, snapshots []*restic.Snapshot) (restic.BlobSet, error) {
	ctx := gopts.ctx

	Verbosef("find data that is still in use for %d snapshots\n", len(snapshots))

	usedBlobs := restic.NewBlobSet()
	//seenBlobs := restic.NewBlobSet()

	bar := newProgressMax(!gopts.Quiet, uint64(len(snapshots)), "snapshots")
	bar.Start()
	for _, sn := range snapshots {
		debug.Log("process snapshot %v", sn.ID())

		err := restic.FindUsedBlobs(ctx, repo, *sn.Tree, usedBlobs, usedBlobs)
		if err != nil {
			if repo.Backend().IsNotExist(err) {
				return nil, errors.Fatal("unable to load a tree from the repo: " + err.Error())
			}

			return nil, err
		}

		debug.Log("processed snapshot %v", sn.ID())
		bar.Report(restic.Stat{Blobs: 1})
	}
	bar.Done()
	return usedBlobs, nil
}
