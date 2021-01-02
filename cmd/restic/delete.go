package main

import (
	"golang.org/x/sync/errgroup"

	"github.com/restic/restic/internal/restic"
)

// DeleteFiles deletes the given fileList of fileType in parallel
// it will print a warning if there is an error, but continue deleting the remaining files
func DeleteFiles(gopts GlobalOptions, repo restic.Repository, fileList restic.IDSet, fileType restic.FileType) {
	deleteFiles(gopts, true, repo, fileList, fileType)
}

// DeleteFilesChecked deletes the given fileList of fileType in parallel
// if an error occurs, it will cancel and return this error
func DeleteFilesChecked(gopts GlobalOptions, repo restic.Repository, fileList restic.IDSet, fileType restic.FileType) error {
	return deleteFiles(gopts, false, repo, fileList, fileType)
}

const numDeleteWorkers = 8

// deleteFiles deletes the given fileList of fileType in parallel
// if ignoreError=true, it will print a warning if there was an error, else it will abort.
func deleteFiles(gopts GlobalOptions, ignoreError bool, repo restic.Repository, fileList restic.IDSet, fileType restic.FileType) error {
	totalCount := len(fileList)
	fileChan := make(chan restic.ID)
	go func() {
		for id := range fileList {
			fileChan <- id
		}
		close(fileChan)
	}()

	bar := newProgressMax(!gopts.JSON && !gopts.Quiet, uint64(totalCount), "files deleted")
	defer bar.Done()
	wg, ctx := errgroup.WithContext(gopts.ctx)
	for i := 0; i < numDeleteWorkers; i++ {
		wg.Go(func() error {
			for id := range fileChan {
				h := restic.Handle{Type: fileType, Name: id.String()}
				err := repo.Backend().Remove(ctx, h)
				if err != nil {
					if !gopts.JSON {
						Warnf("unable to remove %v from the repository\n", h)
					}
					if !ignoreError {
						return err
					}
				}
				if !gopts.JSON && gopts.verbosity > 2 {
					Verbosef("removed %v\n", h)
				}
				bar.Add(1)
			}
			return nil
		})
	}
	err := wg.Wait()
	return err
}
