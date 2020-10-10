// +build debug

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/pack"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
)

var cmdDebug = &cobra.Command{
	Use:   "debug",
	Short: "Debug commands",
}

var cmdDebugDump = &cobra.Command{
	Use:   "dump [indexes|snapshots|all|packs]",
	Short: "Dump data structures",
	Long: `
The "dump" command dumps data structures from the repository as JSON objects. It
is used for debugging purposes only.

EXIT STATUS
===========

Exit status is 0 if the command was successful, and non-zero if there was any error.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDebugDump(globalOptions, args)
	},
}

func init() {
	cmdRoot.AddCommand(cmdDebug)
	cmdDebug.AddCommand(cmdDebugDump)
}

func prettyPrintJSON(wr io.Writer, item interface{}) error {
	buf, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}

	_, err = wr.Write(append(buf, '\n'))
	return err
}

func debugPrintSnapshots(ctx context.Context, repo *repository.Repository, wr io.Writer) error {
	return repo.List(ctx, restic.SnapshotFile, func(id restic.ID, size int64) error {
		snapshot, err := restic.LoadSnapshot(ctx, repo, id)
		if err != nil {
			return err
		}

		fmt.Fprintf(wr, "snapshot_id: %v\n", id)

		return prettyPrintJSON(wr, snapshot)
	})
}

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

func printPacks(ctx context.Context, repo *repository.Repository, wr io.Writer) error {

	return repo.List(ctx, restic.PackFile, func(id restic.ID, size int64) error {
		h := restic.Handle{Type: restic.PackFile, Name: id.String()}

		blobs, err := pack.List(repo.Key(), restic.ReaderAt(ctx, repo.Backend(), h), size)
		if err != nil {
			Warnf("error for pack %v: %v\n", id.Str(), err)
			return nil
		}

		p := Pack{
			Name:  id.String(),
			Blobs: make([]Blob, len(blobs)),
		}
		for i, blob := range blobs {
			p.Blobs[i] = Blob{
				Type:   blob.Type,
				Length: blob.Length,
				ID:     blob.ID,
				Offset: blob.Offset,
			}
		}

		return prettyPrintJSON(wr, p)
	})
}

func dumpIndexes(ctx context.Context, repo restic.Repository, wr io.Writer) error {
	return repo.List(ctx, restic.IndexFile, func(id restic.ID, size int64) error {
		Printf("index_id: %v\n", id)

		idx, err := repository.LoadIndex(ctx, repo, id)
		if err != nil {
			return err
		}

		return idx.Dump(wr)
	})
}

func runDebugDump(gopts GlobalOptions, args []string) error {
	if len(args) != 1 {
		return errors.Fatal("type not specified")
	}

	repo, err := OpenRepository(gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		lock, err := lockRepo(gopts.ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	tpe := args[0]

	switch tpe {
	case "indexes":
		return dumpIndexes(gopts.ctx, repo, gopts.stdout)
	case "snapshots":
		return debugPrintSnapshots(gopts.ctx, repo, gopts.stdout)
	case "packs":
		return printPacks(gopts.ctx, repo, gopts.stdout)
	case "all":
		Printf("snapshots:\n")
		err := debugPrintSnapshots(gopts.ctx, repo, gopts.stdout)
		if err != nil {
			return err
		}

		Printf("\nindexes:\n")
		err = dumpIndexes(gopts.ctx, repo, gopts.stdout)
		if err != nil {
			return err
		}

		return nil
	default:
		return errors.Fatalf("no such type %q", tpe)
	}
}
