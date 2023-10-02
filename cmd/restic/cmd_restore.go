package main

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
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

To only restore a specific subfolder, you can use the "<snapshotID>:<subfolder>"
syntax, where "subfolder" is a path within the snapshot.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		var wg sync.WaitGroup
		cancelCtx, cancel := context.WithCancel(ctx)
		defer func() {
			// shutdown termstatus
			cancel()
			wg.Wait()
		}()

		term := termstatus.New(globalOptions.stdout, globalOptions.stderr, globalOptions.Quiet)
		wg.Add(1)
		go func() {
			defer wg.Done()
			term.Run(cancelCtx)
		}()

		// allow usage of warnf / verbosef
		prevStdout, prevStderr := globalOptions.stdout, globalOptions.stderr
		defer func() {
			globalOptions.stdout, globalOptions.stderr = prevStdout, prevStderr
		}()
		stdioWrapper := ui.NewStdioWrapper(term)
		globalOptions.stdout, globalOptions.stderr = stdioWrapper.Stdout(), stdioWrapper.Stderr()

		return runRestore(ctx, restoreOptions, globalOptions, term, args)
	},
}

// RestoreOptions collects all options for the restore command.
type RestoreOptions struct {
	Exclude            []string
	InsensitiveExclude []string
	Include            []string
	InsensitiveInclude []string
	Target             string
	ScopeSymlinks      string

	restic.SnapshotFilter
	Sparse bool
	Verify bool
}

var restoreOptions RestoreOptions

func init() {
	cmdRoot.AddCommand(cmdRestore)

	flags := cmdRestore.Flags()
	flags.StringArrayVarP(&restoreOptions.Exclude, "exclude", "e", nil, "exclude a `pattern` (can be specified multiple times)")
	flags.StringArrayVar(&restoreOptions.InsensitiveExclude, "iexclude", nil, "same as --exclude but ignores the casing of `pattern`")
	flags.StringArrayVarP(&restoreOptions.Include, "include", "i", nil, "include a `pattern`, exclude everything else (can be specified multiple times)")
	flags.StringArrayVar(&restoreOptions.InsensitiveInclude, "iinclude", nil, "same as --include but ignores the casing of `pattern`")
	flags.StringVarP(&restoreOptions.Target, "target", "t", "", "directory to extract data to")
	flags.StringVar(&restoreOptions.ScopeSymlinks, "scope-symlinks", "", "do not extract symlinks that are targeting files outside this path")

	initSingleSnapshotFilter(flags, &restoreOptions.SnapshotFilter)
	flags.BoolVar(&restoreOptions.Sparse, "sparse", false, "restore files as sparse")
	flags.BoolVar(&restoreOptions.Verify, "verify", false, "verify restored files content")
}

func runRestore(ctx context.Context, opts RestoreOptions, gopts GlobalOptions,
	term *termstatus.Terminal, args []string) error {

	hasExcludes := len(opts.Exclude) > 0 || len(opts.InsensitiveExclude) > 0
	hasIncludes := len(opts.Include) > 0 || len(opts.InsensitiveInclude) > 0
	hasSymlinkScope := opts.ScopeSymlinks != ""

	// Validate provided patterns
	if len(opts.Exclude) > 0 {
		if err := filter.ValidatePatterns(opts.Exclude); err != nil {
			return errors.Fatalf("--exclude: %s", err)
		}
	}
	if len(opts.InsensitiveExclude) > 0 {
		if err := filter.ValidatePatterns(opts.InsensitiveExclude); err != nil {
			return errors.Fatalf("--iexclude: %s", err)
		}
	}
	if len(opts.Include) > 0 {
		if err := filter.ValidatePatterns(opts.Include); err != nil {
			return errors.Fatalf("--include: %s", err)
		}
	}
	if len(opts.InsensitiveInclude) > 0 {
		if err := filter.ValidatePatterns(opts.InsensitiveInclude); err != nil {
			return errors.Fatalf("--iinclude: %s", err)
		}
	}

	for i, str := range opts.InsensitiveExclude {
		opts.InsensitiveExclude[i] = strings.ToLower(str)
	}

	for i, str := range opts.InsensitiveInclude {
		opts.InsensitiveInclude[i] = strings.ToLower(str)
	}

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

	snapshotIDString := args[0]

	debug.Log("restore %v to %v", snapshotIDString, opts.Target)

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		var lock *restic.Lock
		lock, ctx, err = lockRepo(ctx, repo, gopts.RetryLock, gopts.JSON)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	sn, subfolder, err := (&restic.SnapshotFilter{
		Hosts: opts.Hosts,
		Paths: opts.Paths,
		Tags:  opts.Tags,
	}).FindLatest(ctx, repo.Backend(), repo, snapshotIDString)
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
		printer = restoreui.NewJSONProgress(term)
	} else {
		printer = restoreui.NewTextProgress(term)
	}

	progress := restoreui.NewProgress(printer, calculateProgressInterval(!gopts.Quiet, gopts.JSON))
	res := restorer.NewRestorer(repo, sn, opts.Sparse, progress)

	totalErrors := 0
	res.Error = func(location string, err error) error {
		msg.E("ignoring error for %s: %s\n", location, err)
		totalErrors++
		return nil
	}

	excludePatterns := filter.ParsePatterns(opts.Exclude)
	insensitiveExcludePatterns := filter.ParsePatterns(opts.InsensitiveExclude)
	selectExcludeFilter := func(item string, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool) {
		matched, err := filter.List(excludePatterns, item)
		if err != nil {
			msg.E("error for exclude pattern: %v", err)
		}

		matchedInsensitive, err := filter.List(insensitiveExcludePatterns, strings.ToLower(item))
		if err != nil {
			msg.E("error for iexclude pattern: %v", err)
		}

		// An exclude filter is basically a 'wildcard but foo',
		// so even if a childMayMatch, other children of a dir may not,
		// therefore childMayMatch does not matter, but we should not go down
		// unless the dir is selected for restore
		selectedForRestore = !matched && !matchedInsensitive
		childMayBeSelected = selectedForRestore && node.Type == "dir"

		return selectedForRestore, childMayBeSelected
	}

	includePatterns := filter.ParsePatterns(opts.Include)
	insensitiveIncludePatterns := filter.ParsePatterns(opts.InsensitiveInclude)
	selectIncludeFilter := func(item string, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool) {
		matched, childMayMatch, err := filter.ListWithChild(includePatterns, item)
		if err != nil {
			msg.E("error for include pattern: %v", err)
		}

		matchedInsensitive, childMayMatchInsensitive, err := filter.ListWithChild(insensitiveIncludePatterns, strings.ToLower(item))
		if err != nil {
			msg.E("error for iexclude pattern: %v", err)
		}

		selectedForRestore = matched || matchedInsensitive
		childMayBeSelected = (childMayMatch || childMayMatchInsensitive) && node.Type == "dir"

		return selectedForRestore, childMayBeSelected
	}

	symlinkScope := opts.ScopeSymlinks
	selectSymlinkScopeFilter := func(item string, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool) {
		childMayBeSelected = node.Type == "dir"
		if node.Type != "symlink" {
			return true, childMayBeSelected
		}

		// node.LinkTarget can be absolute (e.g. /var/test/target) or:
		// 1. relative, with .. somewhere in the path (e.g. /var/test/target/next/..)
		// 2. relative, starting with . (e.g. ./test/target)
		//
		// Need to clean node.LinkTarget to remove abundant relative path links:
		//   /var/test/target/next/.. -> /var/test/target
		//   ./test/target -> test/target
		//
		// The path can still be relative after Clean (e.g. ./var/../../target -> ../target)
		// so we need to convert it to absolute with destination path in mind.
		// To do this, select the top destination path element that is not a file
		// and append the target to it:
		//   /restore/test/symlink -> /restore/test/../target
		//
		// and then run Clean again to remove remaining relative path links:
		//   /restore/test/../target -> /restore/target
		target := filepath.Clean(node.LinkTarget)
		if !filepath.IsAbs(target) {
			target = filepath.Clean(filepath.Join(filepath.Dir(dstpath), target))
		}

		target, err := filepath.EvalSymlinks(target)
		if err != nil {
			msg.E("error for eval symlink: %v", err)
			// reject symlink if we cannot determine its target
			return false, childMayBeSelected
		}

		return strings.HasPrefix(target, symlinkScope), childMayBeSelected
	}

	selectFilters := []func(item string, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool){}
	if hasExcludes {
		selectFilters = append(selectFilters, selectExcludeFilter)
	} else if hasIncludes {
		selectFilters = append(selectFilters, selectIncludeFilter)
	}

	if hasSymlinkScope {
		selectFilters = append(selectFilters, selectSymlinkScopeFilter)
	}

	if len(selectFilters) > 0 {
		res.SelectFilter = func(item string, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool) {
			for _, filter := range selectFilters {
				selectedForRestore, childMayBeSelected = filter(item, dstpath, node)
				if !selectedForRestore {
					break
				}
			}
			return selectedForRestore, childMayBeSelected
		}
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
