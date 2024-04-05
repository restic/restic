package main

import (
	"context"

	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
)

// DeleteFiles deletes the given fileList of fileType in parallel
// it will print a warning if there is an error, but continue deleting the remaining files
func DeleteFiles(ctx context.Context, repo restic.Repository, fileList restic.IDSet, fileType restic.FileType, printer progress.Printer) {
	_ = deleteFiles(ctx, true, repo, fileList, fileType, printer)
}

// DeleteFilesChecked deletes the given fileList of fileType in parallel
// if an error occurs, it will cancel and return this error
func DeleteFilesChecked(ctx context.Context, repo restic.Repository, fileList restic.IDSet, fileType restic.FileType, printer progress.Printer) error {
	return deleteFiles(ctx, false, repo, fileList, fileType, printer)
}

// deleteFiles deletes the given fileList of fileType in parallel
// if ignoreError=true, it will print a warning if there was an error, else it will abort.
func deleteFiles(ctx context.Context, ignoreError bool, repo restic.Repository, fileList restic.IDSet, fileType restic.FileType, printer progress.Printer) error {
	bar := printer.NewCounter("files deleted")
	defer bar.Done()

	return restic.ParallelRemove(ctx, repo, fileList, fileType, func(id restic.ID, err error) error {
		if err != nil {
			printer.E("unable to remove %v/%v from the repository\n", fileType, id)
			if !ignoreError {
				return err
			}
		}
		printer.VV("removed %v/%v\n", fileType, id)
		return nil
	}, bar)
}
