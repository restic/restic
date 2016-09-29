package main

import (
	"restic"
	"restic/debug"
	"restic/errors"
	"restic/filter"

	"github.com/spf13/cobra"
)

var cmdRestore = &cobra.Command{
	Use:   "restore [flags] snapshotID",
	Short: "extract the data from a snapshot",
	Long: `
The "restore" command extracts the data from a snapshot from the repository to
a directory.

The special snapshot "latest" can be used to restore the latest snapshot in the
repository.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRestore(restoreOptions, globalOptions, args)
	},
}

// RestoreOptions collects all options for the restore command.
type RestoreOptions struct {
	Exclude []string
	Include []string
	Target  string
	Host    string
	Paths   []string
}

var restoreOptions RestoreOptions

func init() {
	cmdRoot.AddCommand(cmdRestore)

	flags := cmdRestore.Flags()
	flags.StringSliceVarP(&restoreOptions.Exclude, "exclude", "e", nil, "exclude a `pattern` (can be specified multiple times)")
	flags.StringSliceVarP(&restoreOptions.Include, "include", "i", nil, "include a `pattern`, exclude everything else (can be specified multiple times)")
	flags.StringVarP(&restoreOptions.Target, "target", "t", "", "directory to extract data to")

	flags.StringVarP(&restoreOptions.Host, "host", "H", "", `only consider snapshots for this host when the snapshot ID is "latest"`)
	flags.StringSliceVar(&restoreOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path` for snapshot ID \"latest\"")
}

func runRestore(opts RestoreOptions, gopts GlobalOptions, args []string) error {
	if len(args) != 1 {
		return errors.Fatalf("no snapshot ID specified")
	}

	if opts.Target == "" {
		return errors.Fatal("please specify a directory to restore to (--target)")
	}

	if len(opts.Exclude) > 0 && len(opts.Include) > 0 {
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

	err = repo.LoadIndex()
	if err != nil {
		return err
	}

	var id restic.ID

	if snapshotIDString == "latest" {
		id, err = restic.FindLatestSnapshot(repo, opts.Paths, opts.Host)
		if err != nil {
			Exitf(1, "latest snapshot for criteria not found: %v Paths:%v Host:%v", err, opts.Paths, opts.Host)
		}
	} else {
		id, err = restic.FindSnapshot(repo, snapshotIDString)
		if err != nil {
			Exitf(1, "invalid id %q: %v", snapshotIDString, err)
		}
	}

	res, err := restic.NewRestorer(repo, id)
	if err != nil {
		Exitf(2, "creating restorer failed: %v\n", err)
	}

	res.Error = func(dir string, node *restic.Node, err error) error {
		Warnf("error for %s: %+v\n", dir, err)
		return nil
	}

	selectExcludeFilter := func(item string, dstpath string, node *restic.Node) bool {
		matched, err := filter.List(opts.Exclude, item)
		if err != nil {
			Warnf("error for exclude pattern: %v", err)
		}

		return !matched
	}

	selectIncludeFilter := func(item string, dstpath string, node *restic.Node) bool {
		matched, err := filter.List(opts.Include, item)
		if err != nil {
			Warnf("error for include pattern: %v", err)
		}

		return matched
	}

	if len(opts.Exclude) > 0 {
		res.SelectFilter = selectExcludeFilter
	} else if len(opts.Include) > 0 {
		res.SelectFilter = selectIncludeFilter
	}

	Verbosef("restoring %s to %s\n", res.Snapshot(), opts.Target)

	return res.RestoreTo(opts.Target)
}
