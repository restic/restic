package main

import (
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/filter"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/restorer"
	"strings"

	"github.com/spf13/cobra"
)

var cmdRestore = &cobra.Command{
	Use:   "restore [flags] snapshotID",
	Short: "Extract the data from a snapshot",
	Long: `
The "restore" command extracts the data from a snapshot from the repository to
a directory.

The special snapshot "latest" can be used to restore the latest snapshot in the
repository.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRestore(restoreOptions, globalOptions, args)
	},
}

// RestoreOptions collects all options for the restore command.
type RestoreOptions struct {
	Exclude            []string
	InsensitiveExclude []string
	Include            []string
	InsensitiveInclude []string
	Target             string
	Host               string
	Paths              []string
	Tags               restic.TagLists
	Verify             bool
}

var restoreOptions RestoreOptions

func init() {
	cmdRoot.AddCommand(cmdRestore)

	flags := cmdRestore.Flags()
	flags.StringArrayVarP(&restoreOptions.Exclude, "exclude", "e", nil, "exclude a `pattern` (can be specified multiple times)")
	flags.StringArrayVar(&restoreOptions.InsensitiveExclude, "iexclude", nil, "same as `--exclude` but ignores the casing of filenames")
	flags.StringArrayVarP(&restoreOptions.Include, "include", "i", nil, "include a `pattern`, exclude everything else (can be specified multiple times)")
	flags.StringArrayVar(&restoreOptions.InsensitiveInclude, "iinclude", nil, "same as `--include` but ignores the casing of filenames")
	flags.StringVarP(&restoreOptions.Target, "target", "t", "", "directory to extract data to")

	flags.StringVarP(&restoreOptions.Host, "host", "H", "", `only consider snapshots for this host when the snapshot ID is "latest"`)
	flags.Var(&restoreOptions.Tags, "tag", "only consider snapshots which include this `taglist` for snapshot ID \"latest\"")
	flags.StringArrayVar(&restoreOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path` for snapshot ID \"latest\"")
	flags.BoolVar(&restoreOptions.Verify, "verify", false, "verify restored files content")
}

func runRestore(opts RestoreOptions, gopts GlobalOptions, args []string) error {
	ctx := gopts.ctx
	hasExcludes := len(opts.Exclude) > 0 || len(opts.InsensitiveExclude) > 0
	hasIncludes := len(opts.Include) > 0 || len(opts.InsensitiveInclude) > 0

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

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		lock, err := lockRepo(repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	err = repo.LoadIndex(ctx)
	if err != nil {
		return err
	}

	var id restic.ID

	if snapshotIDString == "latest" {
		id, err = restic.FindLatestSnapshot(ctx, repo, opts.Paths, opts.Tags, opts.Host)
		if err != nil {
			Exitf(1, "latest snapshot for criteria not found: %v Paths:%v Host:%v", err, opts.Paths, opts.Host)
		}
	} else {
		id, err = restic.FindSnapshot(repo, snapshotIDString)
		if err != nil {
			Exitf(1, "invalid id %q: %v", snapshotIDString, err)
		}
	}

	res, err := restorer.NewRestorer(repo, id)
	if err != nil {
		Exitf(2, "creating restorer failed: %v\n", err)
	}

	totalErrors := 0
	res.Error = func(location string, err error) error {
		Warnf("ignoring error for %s: %s\n", location, err)
		totalErrors++
		return nil
	}

	selectExcludeFilter := func(item string, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool) {
		matched, _, err := filter.List(opts.Exclude, item)
		if err != nil {
			Warnf("error for exclude pattern: %v", err)
		}

		matchedInsensitive, _, err := filter.List(opts.InsensitiveExclude, strings.ToLower(item))
		if err != nil {
			Warnf("error for iexclude pattern: %v", err)
		}

		// An exclude filter is basically a 'wildcard but foo',
		// so even if a childMayMatch, other children of a dir may not,
		// therefore childMayMatch does not matter, but we should not go down
		// unless the dir is selected for restore
		selectedForRestore = !matched && !matchedInsensitive
		childMayBeSelected = selectedForRestore && node.Type == "dir"

		return selectedForRestore, childMayBeSelected
	}

	selectIncludeFilter := func(item string, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool) {
		matched, childMayMatch, err := filter.List(opts.Include, item)
		if err != nil {
			Warnf("error for include pattern: %v", err)
		}

		matchedInsensitive, childMayMatchInsensitive, err := filter.List(opts.InsensitiveInclude, strings.ToLower(item))
		if err != nil {
			Warnf("error for iexclude pattern: %v", err)
		}

		selectedForRestore = matched || matchedInsensitive
		childMayBeSelected = (childMayMatch || childMayMatchInsensitive) && node.Type == "dir"

		return selectedForRestore, childMayBeSelected
	}

	if hasExcludes {
		res.SelectFilter = selectExcludeFilter
	} else if hasIncludes {
		res.SelectFilter = selectIncludeFilter
	}

	Verbosef("restoring %s to %s\n", res.Snapshot(), opts.Target)

	err = res.RestoreTo(ctx, opts.Target)
	if err == nil && opts.Verify {
		Verbosef("verifying files in %s\n", opts.Target)
		var count int
		count, err = res.VerifyFiles(ctx, opts.Target)
		Verbosef("finished verifying %d files in %s\n", count, opts.Target)
	}
	if totalErrors > 0 {
		Printf("There were %d errors\n", totalErrors)
	}
	return err
}
