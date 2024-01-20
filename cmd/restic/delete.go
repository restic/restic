package main

import (
	"context"

	"github.com/restic/restic/internal/restic"
)

// DeleteFiles deletes the given fileList of fileType in parallel
// it will print a warning if there is an error, but continue deleting the remaining files
func DeleteFiles(ctx context.Context, gopts GlobalOptions, repo restic.Repository, fileList restic.IDSet, fileType restic.FileType) {
	_ = deleteFiles(ctx, gopts, true, repo, fileList, fileType)
}

// DeleteFilesChecked deletes the given fileList of fileType in parallel
// if an error occurs, it will cancel and return this error
func DeleteFilesChecked(ctx context.Context, gopts GlobalOptions, repo restic.Repository, fileList restic.IDSet, fileType restic.FileType) error {
	return deleteFiles(ctx, gopts, false, repo, fileList, fileType)
}

// deleteFiles deletes the given fileList of fileType in parallel
// if ignoreError=true, it will print a warning if there was an error, else it will abort.
func deleteFiles(ctx context.Context, gopts GlobalOptions, ignoreError bool, repo restic.Repository, fileList restic.IDSet, fileType restic.FileType) error {
	bar := newProgressMax(!gopts.JSON && !gopts.Quiet, 0, "files deleted")
	defer bar.Done()

	return restic.ParallelRemove(ctx, repo, fileList, fileType, func(id restic.ID, err error) error {
		if err != nil {
			if !gopts.JSON {
				Warnf("unable to remove %v/%v from the repository\n", fileType, id)
			}
			if !ignoreError {
				return err
			}
		}
		if !gopts.JSON && gopts.verbosity > 2 {
			Verbosef("removed %v/%v\n", fileType, id)
		}
		return nil
	}, bar)
}
