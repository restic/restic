package main

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"restic"
	"restic/debug"
	"restic/errors"
	"restic/repository"
)

var cmdFind = &cobra.Command{
	Use:   "find [flags] PATTERN",
	Short: "find a file or directory",
	Long: `
The "find" command searches for files or directories in snapshots stored in the
repo. `,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runFind(findOptions, globalOptions, args)
	},
}

// FindOptions bundles all options for the find command.
type FindOptions struct {
	Oldest          string
	Newest          string
	Snapshots       []string
	CaseInsensitive bool
	ListLong        bool
	Host            string
	Paths           []string
	Tags            []string
}

var findOptions FindOptions

func init() {
	cmdRoot.AddCommand(cmdFind)

	f := cmdFind.Flags()
	f.StringVarP(&findOptions.Oldest, "oldest", "o", "", "oldest modification date/time")
	f.StringVarP(&findOptions.Newest, "newest", "n", "", "newest modification date/time")
	f.StringSliceVarP(&findOptions.Snapshots, "snapshot", "s", nil, "snapshot `id` to search in (can be given multiple times)")
	f.BoolVarP(&findOptions.CaseInsensitive, "ignore-case", "i", false, "ignore case for pattern")
	f.BoolVarP(&findOptions.ListLong, "long", "l", false, "use a long listing format showing size and mode")

	f.StringVarP(&findOptions.Host, "host", "H", "", "only consider snapshots for this `host`, when no snapshot ID is given")
	f.StringSliceVar(&findOptions.Tags, "tag", nil, "only consider snapshots which include this `tag`, when no snapshot-ID is given")
	f.StringSliceVar(&findOptions.Paths, "path", nil, "only consider snapshots which include this (absolute) `path`, when no snapshot-ID is given")
}

type findPattern struct {
	oldest, newest time.Time
	pattern        string
	ignoreCase     bool
}

var timeFormats = []string{
	"2006-01-02",
	"2006-01-02 15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05 MST",
	"02.01.2006",
	"02.01.2006 15:04",
	"02.01.2006 15:04:05",
	"02.01.2006 15:04:05 -0700",
	"02.01.2006 15:04:05 MST",
	"Mon Jan 2 15:04:05 -0700 MST 2006",
}

func parseTime(str string) (time.Time, error) {
	for _, fmt := range timeFormats {
		if t, err := time.ParseInLocation(fmt, str, time.Local); err == nil {
			return t, nil
		}
	}

	return time.Time{}, errors.Fatalf("unable to parse time: %q", str)
}

func findInTree(repo *repository.Repository, pat findPattern, id restic.ID, prefix string, snapshotID *string) error {
	debug.Log("checking tree %v\n", id)

	tree, err := repo.LoadTree(id)
	if err != nil {
		return err
	}

	for _, node := range tree.Nodes {
		debug.Log("  testing entry %q\n", node.Name)

		name := node.Name
		if pat.ignoreCase {
			name = strings.ToLower(name)
		}

		m, err := filepath.Match(pat.pattern, name)
		if err != nil {
			return err
		}

		if m {
			debug.Log("    pattern matches\n")
			if !pat.oldest.IsZero() && node.ModTime.Before(pat.oldest) {
				debug.Log("    ModTime is older than %s\n", pat.oldest)
				continue
			}

			if !pat.newest.IsZero() && node.ModTime.After(pat.newest) {
				debug.Log("    ModTime is newer than %s\n", pat.newest)
				continue
			}

			if snapshotID != nil {
				Verbosef("Found matching entries in snapshot %s\n", *snapshotID)
				snapshotID = nil
			}
			Printf(formatNode(prefix, node, findOptions.ListLong) + "\n")
		} else {
			debug.Log("    pattern does not match\n")
		}

		if node.Type == "dir" {
			if err := findInTree(repo, pat, *node.Subtree, filepath.Join(prefix, node.Name), snapshotID); err != nil {
				return err
			}
		}
	}

	return nil
}

func findInSnapshot(repo *repository.Repository, sn *restic.Snapshot, pat findPattern) error {
	debug.Log("searching in snapshot %s\n  for entries within [%s %s]", sn.ID(), pat.oldest, pat.newest)

	snapshotID := sn.ID().Str()
	if err := findInTree(repo, pat, *sn.Tree, string(filepath.Separator), &snapshotID); err != nil {
		return err
	}
	return nil
}

func runFind(opts FindOptions, gopts GlobalOptions, args []string) error {
	if len(args) != 1 {
		return errors.Fatal("wrong number of arguments")
	}

	var err error
	pat := findPattern{pattern: args[0]}
	if opts.CaseInsensitive {
		pat.pattern = strings.ToLower(pat.pattern)
		pat.ignoreCase = true
	}

	if opts.Oldest != "" {
		if pat.oldest, err = parseTime(opts.Oldest); err != nil {
			return err
		}
	}

	if opts.Newest != "" {
		if pat.newest, err = parseTime(opts.Newest); err != nil {
			return err
		}
	}

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

	if err = repo.LoadIndex(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(gopts.ctx)
	defer cancel()
	for sn := range FindFilteredSnapshots(ctx, repo, opts.Host, opts.Tags, opts.Paths, opts.Snapshots) {
		if err = findInSnapshot(repo, sn, pat); err != nil {
			return err
		}
	}

	return nil
}
