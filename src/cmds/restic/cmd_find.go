package main

import (
	"path/filepath"
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

// FindOptions bundle all options for the find command.
type FindOptions struct {
	Oldest   string
	Newest   string
	Snapshot string
}

var findOptions FindOptions

func init() {
	cmdRoot.AddCommand(cmdFind)

	f := cmdFind.Flags()
	f.StringVarP(&findOptions.Oldest, "oldest", "o", "", "Oldest modification date/time")
	f.StringVarP(&findOptions.Newest, "newest", "n", "", "Newest modification date/time")
	f.StringVarP(&findOptions.Snapshot, "snapshot", "s", "", "Snapshot ID to search in")
}

type findPattern struct {
	oldest, newest time.Time
	pattern        string
}

type findResult struct {
	node *restic.Node
	path string
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

func findInTree(repo *repository.Repository, pat findPattern, id restic.ID, path string) ([]findResult, error) {
	debug.Log("checking tree %v\n", id)
	tree, err := repo.LoadTree(id)
	if err != nil {
		return nil, err
	}

	results := []findResult{}
	for _, node := range tree.Nodes {
		debug.Log("  testing entry %q\n", node.Name)

		m, err := filepath.Match(pat.pattern, node.Name)
		if err != nil {
			return nil, err
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

			results = append(results, findResult{node: node, path: path})
		} else {
			debug.Log("    pattern does not match\n")
		}

		if node.Type == "dir" {
			subdirResults, err := findInTree(repo, pat, *node.Subtree, filepath.Join(path, node.Name))
			if err != nil {
				return nil, err
			}

			results = append(results, subdirResults...)
		}
	}

	return results, nil
}

func findInSnapshot(repo *repository.Repository, pat findPattern, id restic.ID) error {
	debug.Log("searching in snapshot %s\n  for entries within [%s %s]", id.Str(), pat.oldest, pat.newest)

	sn, err := restic.LoadSnapshot(repo, id)
	if err != nil {
		return err
	}

	results, err := findInTree(repo, pat, *sn.Tree, "")
	if err != nil {
		return err
	}

	if len(results) == 0 {
		return nil
	}
	Verbosef("found %d matching entries in snapshot %s\n", len(results), id)
	for _, res := range results {
		res.node.Name = filepath.Join(res.path, res.node.Name)
		Printf("  %s\n", res.node)
	}

	return nil
}

func runFind(opts FindOptions, gopts GlobalOptions, args []string) error {
	if len(args) != 1 {
		return errors.Fatalf("wrong number of arguments")
	}

	var (
		err error
		pat findPattern
	)

	if opts.Oldest != "" {
		pat.oldest, err = parseTime(opts.Oldest)
		if err != nil {
			return err
		}
	}

	if opts.Newest != "" {
		pat.newest, err = parseTime(opts.Newest)
		if err != nil {
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

	err = repo.LoadIndex()
	if err != nil {
		return err
	}

	pat.pattern = args[0]

	if opts.Snapshot != "" {
		snapshotID, err := restic.FindSnapshot(repo, opts.Snapshot)
		if err != nil {
			return errors.Fatalf("invalid id %q: %v", args[1], err)
		}

		return findInSnapshot(repo, pat, snapshotID)
	}

	done := make(chan struct{})
	defer close(done)
	for snapshotID := range repo.List(restic.SnapshotFile, done) {
		err := findInSnapshot(repo, pat, snapshotID)

		if err != nil {
			return err
		}
	}

	return nil
}
