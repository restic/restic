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
	Oldest time.Time `short:"o" long:"oldest" description:"Oldest modification date/time"`
	Newest time.Time `short:"n" long:"newest" description:"Newest modification date/time"`
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

func findInTree(ch *restic.ContentHandler, id backend.ID, path, pattern string) ([]findResult, error) {
	debug("checking tree %v\n", id)

	tree, err := restic.LoadTree(ch, id)
	if err != nil {
		return nil, err
	}

	results := []findResult{}
	for _, node := range tree {
		m, err := filepath.Match(pattern, node.Name)
		if err != nil {
			return nil, err
		}

		debug("  testing entry %q: %v\n", node.Name, m)

		if m {
			results = append(results, findResult{node: node, path: path})
		}

		if node.Type == "dir" {
			subdirResults, err := findInTree(ch, node.Subtree, filepath.Join(path, node.Name), pattern)
			if err != nil {
				return nil, err
			}

			results = append(results, subdirResults...)
		}
	}

	return results, nil
}

func findInSnapshot(be backend.Server, key *restic.Key, id backend.ID, pattern string) error {
	debug("searching in snapshot %v\n", id)

	ch, err := restic.NewContentHandler(be, key)
	if err != nil {
		return err
	}

	sn, err := ch.LoadSnapshot(id)
	if err != nil {
		return err
	}

	results, err := findInTree(ch, sn.Tree, "", pattern)
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

func (cmd CmdFind) Usage() string {
	return "[find-OPTIONS] PATTERN [snapshot-ID]"
}

func (cmd CmdFind) Execute(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no pattern given, Usage: %s", cmd.Usage())
	}

	be, key, err := OpenRepo()
	if err != nil {
		return err
	}

	pattern := args[0]
	if len(args) == 2 {
		snapshotID, err := backend.FindSnapshot(be, args[1])
		if err != nil {
			return fmt.Errorf("invalid id %q: %v", args[1], err)
		}

		return findInSnapshot(be, key, snapshotID, pattern)
	}

	list, err := be.List(backend.Snapshot)
	if err != nil {
		return err
	}

	for _, snapshotID := range list {
		err := findInSnapshot(be, key, snapshotID, pattern)

		if err != nil {
			return err
		}
	}

	return nil
}
