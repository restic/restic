package main

import (
	"fmt"
	"path/filepath"
	"time"

	"restic"
	"restic/backend"
	"restic/debug"
	"restic/repository"
)

type findResult struct {
	node *restic.Node
	path string
}

type CmdFind struct {
	Oldest   string `short:"o" long:"oldest" description:"Oldest modification date/time"`
	Newest   string `short:"n" long:"newest" description:"Newest modification date/time"`
	Snapshot string `short:"s" long:"snapshot" description:"Snapshot ID to search in"`

	oldest, newest time.Time
	pattern        string
	global         *GlobalOptions
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

func init() {
	_, err := parser.AddCommand("find",
		"find a file/directory",
		"The find command searches for files or directories in snapshots",
		&CmdFind{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func parseTime(str string) (time.Time, error) {
	for _, fmt := range timeFormats {
		if t, err := time.ParseInLocation(fmt, str, time.Local); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse time: %q", str)
}

func (c CmdFind) findInTree(repo *repository.Repository, id backend.ID, path string) ([]findResult, error) {
	debug.Log("restic.find", "checking tree %v\n", id)
	tree, err := restic.LoadTree(repo, id)
	if err != nil {
		return nil, err
	}

	results := []findResult{}
	for _, node := range tree.Nodes {
		debug.Log("restic.find", "  testing entry %q\n", node.Name)

		m, err := filepath.Match(c.pattern, node.Name)
		if err != nil {
			return nil, err
		}

		if m {
			debug.Log("restic.find", "    pattern matches\n")
			if !c.oldest.IsZero() && node.ModTime.Before(c.oldest) {
				debug.Log("restic.find", "    ModTime is older than %s\n", c.oldest)
				continue
			}

			if !c.newest.IsZero() && node.ModTime.After(c.newest) {
				debug.Log("restic.find", "    ModTime is newer than %s\n", c.newest)
				continue
			}

			results = append(results, findResult{node: node, path: path})
		} else {
			debug.Log("restic.find", "    pattern does not match\n")
		}

		if node.Type == "dir" {
			subdirResults, err := c.findInTree(repo, *node.Subtree, filepath.Join(path, node.Name))
			if err != nil {
				return nil, err
			}

			results = append(results, subdirResults...)
		}
	}

	return results, nil
}

func (c CmdFind) findInSnapshot(repo *repository.Repository, id backend.ID) error {
	debug.Log("restic.find", "searching in snapshot %s\n  for entries within [%s %s]", id.Str(), c.oldest, c.newest)

	sn, err := restic.LoadSnapshot(repo, id)
	if err != nil {
		return err
	}

	results, err := c.findInTree(repo, *sn.Tree, "")
	if err != nil {
		return err
	}

	if len(results) == 0 {
		return nil
	}
	c.global.Verbosef("found %d matching entries in snapshot %s\n", len(results), id)
	for _, res := range results {
		res.node.Name = filepath.Join(res.path, res.node.Name)
		c.global.Printf("  %s\n", res.node)
	}

	return nil
}

func (CmdFind) Usage() string {
	return "[find-OPTIONS] PATTERN"
}

func (c CmdFind) Execute(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("wrong number of arguments, Usage: %s", c.Usage())
	}

	var err error

	if c.Oldest != "" {
		c.oldest, err = parseTime(c.Oldest)
		if err != nil {
			return err
		}
	}

	if c.Newest != "" {
		c.newest, err = parseTime(c.Newest)
		if err != nil {
			return err
		}
	}

	repo, err := c.global.OpenRepository()
	if err != nil {
		return err
	}

	lock, err := lockRepo(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	err = repo.LoadIndex()
	if err != nil {
		return err
	}

	c.pattern = args[0]

	if c.Snapshot != "" {
		snapshotID, err := restic.FindSnapshot(repo, c.Snapshot)
		if err != nil {
			return fmt.Errorf("invalid id %q: %v", args[1], err)
		}

		return c.findInSnapshot(repo, snapshotID)
	}

	done := make(chan struct{})
	defer close(done)
	for snapshotID := range repo.List(backend.Snapshot, done) {
		err := c.findInSnapshot(repo, snapshotID)

		if err != nil {
			return err
		}
	}

	return nil
}
