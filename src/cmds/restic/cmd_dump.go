// +build debug

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"restic"
	"restic/backend"
	"restic/pack"
	"restic/repository"

	"restic/worker"

	"github.com/juju/errors"
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
	return "[indexes|snapshots|trees|all|packs]"
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

	for _, idx := range repo.Index().All() {
		for blob := range idx.Each(nil) {
			if blob.Type != pack.Tree {
				continue
			}

			trees = append(trees, blob.ID)
		}
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

const numWorkers = 10

// Pack is the struct used in printPacks.
type Pack struct {
	Name string `json:"name"`

	Blobs []Blob `json:"blobs"`
}

// Blob is the struct used in printPacks.
type Blob struct {
	Type   pack.BlobType `json:"type"`
	Length uint          `json:"length"`
	ID     backend.ID    `json:"id"`
	Offset uint          `json:"offset"`
}

func printPacks(repo *repository.Repository, wr io.Writer) error {
	done := make(chan struct{})
	defer close(done)

	f := func(job worker.Job, done <-chan struct{}) (interface{}, error) {
		name := job.Data.(string)

		h := backend.Handle{Type: backend.Data, Name: name}
		rd := backend.NewReadSeeker(repo.Backend(), h)

		unpacker, err := pack.NewUnpacker(repo.Key(), rd)
		if err != nil {
			return nil, err
		}

		return unpacker.Entries, nil
	}

	jobCh := make(chan worker.Job)
	resCh := make(chan worker.Job)
	wp := worker.New(numWorkers, f, jobCh, resCh)

	go func() {
		for name := range repo.Backend().List(backend.Data, done) {
			jobCh <- worker.Job{Data: name}
		}
		close(jobCh)
	}()

	for job := range resCh {
		name := job.Data.(string)

		if job.Error != nil {
			fmt.Fprintf(os.Stderr, "error for pack %v: %v\n", name, job.Error)
			continue
		}

		entries := job.Result.([]pack.Blob)
		p := Pack{
			Name:  name,
			Blobs: make([]Blob, len(entries)),
		}
		for i, blob := range entries {
			p.Blobs[i] = Blob{
				Type:   blob.Type,
				Length: blob.Length,
				ID:     blob.ID,
				Offset: blob.Offset,
			}
		}

		prettyPrintJSON(os.Stdout, p)
	}

	wp.Wait()

	return nil
}

func (cmd CmdDump) DumpIndexes() error {
	done := make(chan struct{})
	defer close(done)

	for id := range cmd.repo.List(backend.Index, done) {
		fmt.Printf("index_id: %v\n", id)

		idx, err := repository.LoadIndex(cmd.repo, id)
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
	case "indexes":
		return cmd.DumpIndexes()
	case "snapshots":
		return printSnapshots(repo, os.Stdout)
	case "trees":
		return printTrees(repo, os.Stdout)
	case "packs":
		return printPacks(repo, os.Stdout)
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

		return nil
	default:
		return errors.Errorf("no such type %q", tpe)
	}
}
