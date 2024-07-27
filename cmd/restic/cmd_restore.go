package main

import (
	"context"
	"path/filepath"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/restorer"
	"github.com/restic/restic/internal/ui"
	restoreui "github.com/restic/restic/internal/ui/restore"
	"github.com/restic/restic/internal/ui/termstatus"

	"github.com/spf13/cobra"
)

var cmdRestore = &cobra.Command{
	Use:   "restore [flags] snapshotID",
	Short: "Extract the data from a snapshot",
	Long: `
The "restore" command extracts the data from a snapshot from the repository to
a directory.

The special snapshotID "latest" can be used to restore the latest snapshot in the
repository.

To only restore a specific subfolder, you can use the "snapshotID:subfolder"
syntax, where "subfolder" is a path within the snapshot.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		term, cancel := setupTermstatus()
		defer cancel()
		return runRestore(cmd.Context(), restoreOptions, globalOptions, term, args)
	},
}

// RestoreOptions collects all options for the restore command.
type RestoreOptions struct {
	excludePatternOptions
	includePatternOptions
	Target string
	restic.SnapshotFilter
	DryRun    bool
	Sparse    bool
	Verify    bool
	Overwrite restorer.OverwriteBehavior
	Delete    bool
}

var restoreOptions RestoreOptions

func init() {
	cmdRoot.AddCommand(cmdRestore)

	flags := cmdRestore.Flags()
	flags.StringVarP(&restoreOptions.Target, "target", "t", "", "directory to extract data to")

	initExcludePatternOptions(flags, &restoreOptions.excludePatternOptions)
	initIncludePatternOptions(flags, &restoreOptions.includePatternOptions)

	initSingleSnapshotFilter(flags, &restoreOptions.SnapshotFilter)
	flags.BoolVar(&restoreOptions.DryRun, "dry-run", false, "do not write any data, just show what would be done")
	flags.BoolVar(&restoreOptions.Sparse, "sparse", false, "restore files as sparse")
	flags.BoolVar(&restoreOptions.Verify, "verify", false, "verify restored files content")
	flags.Var(&restoreOptions.Overwrite, "overwrite", "overwrite behavior, one of (always|if-changed|if-newer|never) (default: always)")
	flags.BoolVar(&restoreOptions.Delete, "delete", false, "delete files from target directory if they do not exist in snapshot. Use '--dry-run -vv' to check what would be deleted")
}

func runRestore(ctx context.Context, opts RestoreOptions, gopts GlobalOptions,
	term *termstatus.Terminal, args []string) error {

	excludePatternFns, err := opts.excludePatternOptions.CollectPatterns()
	if err != nil {
		return err
	}

	includePatternFns, err := opts.includePatternOptions.CollectPatterns()
	if err != nil {
		return err
	}

	hasExcludes := len(excludePatternFns) > 0
	hasIncludes := len(includePatternFns) > 0

	switch {
	case len(args) == 0:
		return errors.Fatal("no snapshot ID specified")
	case len(args) > 1:
		return errors.Fatalf("more than one snapshot ID specified: %v", args)
	}

	if opts.Target == "" {
		return errors.Fatal("please specify a directory to restore to (--target)")
	}

	if hasExcludes && hasIncludes {
		return errors.Fatal("exclude and include patterns are mutually exclusive")
	}
	if opts.DryRun && opts.Verify {
		return errors.Fatal("--dry-run and --verify are mutually exclusive")
	}

	if opts.Delete && filepath.Clean(opts.Target) == "/" && !hasExcludes && !hasIncludes {
		return errors.Fatal("'--target / --delete' must be combined with an include or exclude filter")
	}

	snapshotIDString := args[0]

	debug.Log("restore %v to %v", snapshotIDString, opts.Target)

	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock)
	if err != nil {
		return err
	}
	defer unlock()

	sn, subfolder, err := (&restic.SnapshotFilter{
		Hosts: opts.Hosts,
		Paths: opts.Paths,
		Tags:  opts.Tags,
	}).FindLatest(ctx, repo, repo, snapshotIDString)
	if err != nil {
		return errors.Fatalf("failed to find snapshot: %v", err)
	}

	bar := newIndexTerminalProgress(gopts.Quiet, gopts.JSON, term)
	err = repo.LoadIndex(ctx, bar)
	if err != nil {
		return err
	}

	sn.Tree, err = restic.FindTreeDirectory(ctx, repo, sn.Tree, subfolder)
	if err != nil {
		return err
	}

	msg := ui.NewMessage(term, gopts.verbosity)
	var printer restoreui.ProgressPrinter
	if gopts.JSON {
		printer = restoreui.NewJSONProgress(term, gopts.verbosity)
	} else {
		printer = restoreui.NewTextProgress(term, gopts.verbosity)
	}

	progress := restoreui.NewProgress(printer, calculateProgressInterval(!gopts.Quiet, gopts.JSON))
	res := restorer.NewRestorer(repo, sn, restorer.Options{
		DryRun:    opts.DryRun,
		Sparse:    opts.Sparse,
		Progress:  progress,
		Overwrite: opts.Overwrite,
		Delete:    opts.Delete,
	})

	totalErrors := 0
	res.Error = func(location string, err error) error {
		totalErrors++
		return progress.Error(location, err)
	}
	res.Warn = func(message string) {
		msg.E("Warning: %s\n", message)
	}

	selectExcludeFilter := func(item string, isDir bool) (selectedForRestore bool, childMayBeSelected bool) {
		matched := false
		for _, rejectFn := range excludePatternFns {
			matched = matched || rejectFn(item)

			// implementing a short-circuit here to improve the performance
			// to prevent additional pattern matching once the first pattern
			// matches.
			if matched {
				break
			}
		}
		// An exclude filter is basically a 'wildcard but foo',
		// so even if a childMayMatch, other children of a dir may not,
		// therefore childMayMatch does not matter, but we should not go down
		// unless the dir is selected for restore
		selectedForRestore = !matched
		childMayBeSelected = selectedForRestore && isDir

		return selectedForRestore, childMayBeSelected
	}

	selectIncludeFilter := func(item string, isDir bool) (selectedForRestore bool, childMayBeSelected bool) {
		selectedForRestore = false
		childMayBeSelected = false
		for _, includeFn := range includePatternFns {
			matched, childMayMatch := includeFn(item)
			selectedForRestore = selectedForRestore || matched
			childMayBeSelected = childMayBeSelected || childMayMatch

			if selectedForRestore && childMayBeSelected {
				break
			}
		}
		childMayBeSelected = childMayBeSelected && isDir

		return selectedForRestore, childMayBeSelected
	}

	if hasExcludes {
		res.SelectFilter = selectExcludeFilter
	} else if hasIncludes {
		res.SelectFilter = selectIncludeFilter
	}

	if !gopts.JSON {
		msg.P("restoring %s to %s\n", res.Snapshot(), opts.Target)
	}

	err = res.RestoreTo(ctx, opts.Target)
	if err != nil {
		return err
	}

	progress.Finish()

	if totalErrors > 0 {
		return errors.Fatalf("There were %d errors\n", totalErrors)
	}

	if opts.Verify {
		if !gopts.JSON {
			msg.P("verifying files in %s\n", opts.Target)
		}
		var count int
		t0 := time.Now()
		count, err = res.VerifyFiles(ctx, opts.Target)
		if err != nil {
			return err
		}
		if totalErrors > 0 {
			return errors.Fatalf("There were %d errors\n", totalErrors)
		}

		if !gopts.JSON {
			msg.P("finished verifying %d files in %s (took %s)\n", count, opts.Target,
				time.Since(t0).Round(time.Millisecond))
		}
	}

	return nil
}
