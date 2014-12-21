package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
)

type findQuery struct {
	name       string
	minModTime time.Time
	maxModTime time.Time
}

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
		&CmdFind{})
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

func (c CmdFind) findInTree(ch *restic.ContentHandler, id backend.ID, path string) ([]findResult, error) {
	debug("checking tree %v\n", id)

	tree, err := restic.LoadTree(ch, id)
	if err != nil {
		return nil, err
	}

	results := []findResult{}
	for _, node := range tree {
		debug("  testing entry %q\n", node.Name)

		m, err := filepath.Match(c.pattern, node.Name)
		if err != nil {
			return nil, err
		}

		if m {
			debug("    pattern matches\n")
			if !c.oldest.IsZero() && node.ModTime.Before(c.oldest) {
				debug("    ModTime is older than %s\n", c.oldest)
				continue
			}

			if !c.newest.IsZero() && node.ModTime.After(c.newest) {
				debug("    ModTime is newer than %s\n", c.newest)
				continue
			}

			results = append(results, findResult{node: node, path: path})
		} else {
			debug("    pattern does not match\n")
		}

		if node.Type == "dir" {
			subdirResults, err := c.findInTree(ch, node.Subtree, filepath.Join(path, node.Name))
			if err != nil {
				return nil, err
			}

			results = append(results, subdirResults...)
		}
	}

	return results, nil
}

func (c CmdFind) findInSnapshot(s restic.Server, id backend.ID) error {
	debug("searching in snapshot %s\n  for entries within [%s %s]", id, c.oldest, c.newest)

	ch, err := restic.NewContentHandler(s)
	if err != nil {
		return err
	}

	sn, err := ch.LoadSnapshot(id)
	if err != nil {
		return err
	}

	results, err := c.findInTree(ch, sn.Tree, "")
	if err != nil {
		return err
	}

	if len(results) == 0 {
		return nil
	}

	fmt.Printf("found %d matching entries in snapshot %s\n", len(results), id)
	for _, res := range results {
		res.node.Name = filepath.Join(res.path, res.node.Name)
		fmt.Printf("  %s\n", res.node)
	}

	return nil
}

func (CmdFind) Usage() string {
	return "[find-OPTIONS] PATTERN"
}

func (c CmdFind) Execute(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("invalid number of arguments, Usage: %s", c.Usage())
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

	s, err := OpenRepo()
	if err != nil {
		return err
	}

	c.pattern = args[0]

	if c.Snapshot != "" {
		snapshotID, err := backend.FindSnapshot(s, c.Snapshot)
		if err != nil {
			return fmt.Errorf("invalid id %q: %v", args[1], err)
		}

		return c.findInSnapshot(s, snapshotID)
	}

	list, err := s.List(backend.Snapshot)
	if err != nil {
		return err
	}

	for _, snapshotID := range list {
		err := c.findInSnapshot(s, snapshotID)

		if err != nil {
			return err
		}
	}

	return nil
}
