// +build debug

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"restic"
	"restic/errors"
	"restic/pack"
	"restic/repository"

	"restic/worker"
)

var cmdDump = &cobra.Command{
	Use:   "dump [indexes|snapshots|trees|all|packs]",
	Short: "dump data structures",
	Long: `
The "dump" command dumps data structures from a repository as JSON objects. It
is used for debugging purposes only.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDump(globalOptions, args)
	},
}

func init() {
	cmdRoot.AddCommand(cmdDump)
}

func prettyPrintJSON(wr io.Writer, item interface{}) error {
	buf, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}

	_, err = wr.Write(append(buf, '\n'))
	return err
}

func debugPrintSnapshots(repo *repository.Repository, wr io.Writer) error {
	done := make(chan struct{})
	defer close(done)

	for id := range repo.List(restic.SnapshotFile, done) {
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

const dumpPackWorkers = 10

// Pack is the struct used in printPacks.
type Pack struct {
	Name string `json:"name"`

	Blobs []Blob `json:"blobs"`
}

// Blob is the struct used in printPacks.
type Blob struct {
	Type   restic.BlobType `json:"type"`
	Length uint            `json:"length"`
	ID     restic.ID       `json:"id"`
	Offset uint            `json:"offset"`
}

func printPacks(repo *repository.Repository, wr io.Writer) error {
	done := make(chan struct{})
	defer close(done)

	f := func(job worker.Job, done <-chan struct{}) (interface{}, error) {
		name := job.Data.(string)

		h := restic.Handle{Type: restic.DataFile, Name: name}

		blobInfo, err := repo.Backend().Stat(h)
		if err != nil {
			return nil, err
		}

		blobs, err := pack.List(repo.Key(), restic.ReaderAt(repo.Backend(), h), blobInfo.Size)
		if err != nil {
			return nil, err
		}

		return blobs, nil
	}

	jobCh := make(chan worker.Job)
	resCh := make(chan worker.Job)
	wp := worker.New(dumpPackWorkers, f, jobCh, resCh)

	go func() {
		for name := range repo.Backend().List(restic.DataFile, done) {
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

		entries := job.Result.([]restic.Blob)
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

func dumpIndexes(repo restic.Repository) error {
	done := make(chan struct{})
	defer close(done)

	for id := range repo.List(restic.IndexFile, done) {
		fmt.Printf("index_id: %v\n", id)

		idx, err := repository.LoadIndex(repo, id)
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

func runDump(gopts GlobalOptions, args []string) error {
	if len(args) != 1 {
		return errors.Fatalf("type not specified")
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

	tpe := args[0]

	switch tpe {
	case "indexes":
		return dumpIndexes(repo)
	case "snapshots":
		return debugPrintSnapshots(repo, os.Stdout)
	case "packs":
		return printPacks(repo, os.Stdout)
	case "all":
		fmt.Printf("snapshots:\n")
		err := debugPrintSnapshots(repo, os.Stdout)
		if err != nil {
			return err
		}

		fmt.Printf("\nindexes:\n")
		err = dumpIndexes(repo)
		if err != nil {
			return err
		}

		return nil
	default:
		return errors.Fatalf("no such type %q", tpe)
	}
}
