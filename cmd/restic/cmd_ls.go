package main

import (
	"context"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

var cmdLs = &cobra.Command{
	Use:   "ls [flags] [snapshot-ID ...]",
	Short: "List files in a snapshot",
	Long: `
The "ls" command allows listing files and directories in a snapshot.

The special snapshot-ID "latest" can be used to list files and directories of the latest snapshot in the repository.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLs(lsOptions, globalOptions, args)
	},
}

// LsOptions collects all options for the ls command.
type LsOptions struct {
	ListLong bool
	Host     string
	Tags     restic.TagLists
	Paths    []string
}

var lsOptions LsOptions

func init() {
	cmdRoot.AddCommand(cmdLs)

	flags := cmdLs.Flags()
	flags.BoolVarP(&lsOptions.ListLong, "long", "l", false, "use a long listing format showing size and mode")

	flags.StringVarP(&lsOptions.Host, "host", "H", "", "only consider snapshots for this `host`, when no snapshot ID is given")
	flags.Var(&lsOptions.Tags, "tag", "only consider snapshots which include this `taglist`, when no snapshot ID is given")
	flags.StringArrayVar(&lsOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path`, when no snapshot ID is given")
}

func printTree(ctx context.Context, repo *repository.Repository, id *restic.ID, prefix string) error {
	tree, err := repo.LoadTree(ctx, *id)
	if err != nil {
		return err
	}

	for _, entry := range tree.Nodes {
		Printf("%s\n", formatNode(prefix, entry, lsOptions.ListLong))

		if entry.Type == "dir" && entry.Subtree != nil {
			entryPath := prefix + string(filepath.Separator) + entry.Name
			if err = printTree(ctx, repo, entry.Subtree, entryPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func runLs(opts LsOptions, gopts GlobalOptions, args []string) error {
	if len(args) == 0 && opts.Host == "" && len(opts.Tags) == 0 && len(opts.Paths) == 0 {
		return errors.Fatal("Invalid arguments, either give one or more snapshot IDs or set filters.")
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if err = repo.LoadIndex(gopts.ctx); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()
	for sn := range FindFilteredSnapshots(ctx, repo, opts.Host, opts.Tags, opts.Paths, args) {
		Verbosef("snapshot %s of %v at %s):\n", sn.ID().Str(), sn.Paths, sn.Time)

		if err = printTree(gopts.ctx, repo, sn.Tree, ""); err != nil {
			return err
		}
	}
	return nil
}
