// +build debug

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/juju/errors"
	"github.com/restic/restic"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/pack"
	"github.com/restic/restic/repository"
)

type CmdDump struct {
	global *GlobalOptions

	repo *repository.Repository
}

func init() {
	_, err := parser.AddCommand("dump",
		"dump data structures",
		"The dump command dumps data structures from a repository as JSON documents",
		&CmdDump{global: &globalOpts})
	if err != nil {
		panic(err)
	}
}

func (cmd CmdDump) Usage() string {
	return "[index|snapshots|trees|all]"
}

func prettyPrintJSON(wr io.Writer, item interface{}) error {
	buf, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}

	_, err = wr.Write(append(buf, '\n'))
	return err
}

func printSnapshots(repo *repository.Repository, wr io.Writer) error {
	done := make(chan struct{})
	defer close(done)

	for id := range repo.List(backend.Snapshot, done) {
		snapshot, err := restic.LoadSnapshot(repo, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "LoadSnapshot(%v): %v", id.Str(), err)
			continue
		}

		fmt.Fprintf(wr, "snapshot_id: %v\n", id)

		err = prettyPrintJSON(wr, snapshot)
		if err != nil {
			return err
		}
	}

	return nil
}

func printTrees(repo *repository.Repository, wr io.Writer) error {
	done := make(chan struct{})
	defer close(done)

	trees := []backend.ID{}

	for blob := range repo.Index().Each(done) {
		if blob.Type != pack.Tree {
			continue
		}

		trees = append(trees, blob.ID)
	}

	for _, id := range trees {
		tree, err := restic.LoadTree(repo, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "LoadTree(%v): %v", id.Str(), err)
			continue
		}

		fmt.Fprintf(wr, "tree_id: %v\n", id)

		prettyPrintJSON(wr, tree)
	}

	return nil
}

func (cmd CmdDump) DumpIndexes() error {
	done := make(chan struct{})
	defer close(done)

	for id := range cmd.repo.List(backend.Index, done) {
		fmt.Printf("index_id: %v\n", id)

		idx, err := repository.LoadIndex(cmd.repo, id.String())
		if err != nil {
			return err
		}

		err = idx.Dump(os.Stdout)
		if err != nil {
			return err
		}
	}

	return nil
}

func (cmd CmdDump) Execute(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("type not specified, Usage: %s", cmd.Usage())
	}

	repo, err := cmd.global.OpenRepository()
	if err != nil {
		return err
	}
	cmd.repo = repo

	lock, err := lockRepo(repo)
	defer unlockRepo(lock)
	if err != nil {
		return err
	}

	err = repo.LoadIndex()
	if err != nil {
		return err
	}

	tpe := args[0]

	switch tpe {
	case "index":
		return repo.Index().Dump(os.Stdout)
	case "indexes":
		return cmd.DumpIndexes()
	case "snapshots":
		return printSnapshots(repo, os.Stdout)
	case "trees":
		return printTrees(repo, os.Stdout)
	case "all":
		fmt.Printf("snapshots:\n")
		err := printSnapshots(repo, os.Stdout)
		if err != nil {
			return err
		}

		fmt.Printf("\ntrees:\n")

		err = printTrees(repo, os.Stdout)
		if err != nil {
			return err
		}

		fmt.Printf("\nindexes:\n")
		err = cmd.DumpIndexes()
		if err != nil {
			return err
		}

		fmt.Printf("\nindex:\n")

		err = repo.Index().Dump(os.Stdout)
		if err != nil {
			return err
		}

		return nil
	default:
		return errors.Errorf("no such type %q", tpe)
	}
}
