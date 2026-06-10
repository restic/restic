//go:build debug

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/global"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui"
	"github.com/restic/restic/internal/ui/progress"
)

func registerDebugCommand(cmd *cobra.Command, globalOptions *global.Options) {
	cmd.AddCommand(
		newDebugCommand(globalOptions),
	)
}

func newDebugCommand(globalOptions *global.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "debug",
		Short:             "Debug commands",
		GroupID:           cmdGroupDefault,
		DisableAutoGenTag: true,
	}
	cmd.AddCommand(newDebugDumpCommand(globalOptions))
	cmd.AddCommand(newDebugExamineCommand(globalOptions))
	return cmd
}

func newDebugDumpCommand(globalOptions *global.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dump [indexes|snapshots|all|packs]",
		Short: "Dump data structures",
		Long: `
The "dump" command dumps data structures from the repository as JSON objects. It
is used for debugging purposes only.

EXIT STATUS
===========

Exit status is 0 if the command was successful.
Exit status is 1 if there was any error.
Exit status is 10 if the repository does not exist.
Exit status is 11 if the repository is already locked.
Exit status is 12 if the password is incorrect.
`,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDebugDump(cmd.Context(), *globalOptions, args, globalOptions.Term)
		},
	}
	return cmd
}

func newDebugExamineCommand(globalOptions *global.Options) *cobra.Command {
	var opts DebugExamineOptions

	cmd := &cobra.Command{
		Use:               "examine pack-ID...",
		Short:             "Examine a pack file",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDebugExamine(cmd.Context(), *globalOptions, opts, args, globalOptions.Term)
		},
	}

	opts.AddFlags(cmd.Flags())
	return cmd
}

type DebugExamineOptions struct {
	TryRepair     bool
	RepairByte    bool
	ExtractPack   bool
	ReuploadBlobs bool
}

func (opts *DebugExamineOptions) AddFlags(f *pflag.FlagSet) {
	f.BoolVar(&opts.ExtractPack, "extract-pack", false, "write blobs to the current directory")
	f.BoolVar(&opts.ReuploadBlobs, "reupload-blobs", false, "reupload blobs to the repository")
	f.BoolVar(&opts.TryRepair, "try-repair", false, "try to repair broken blobs with single bit flips")
	f.BoolVar(&opts.RepairByte, "repair-byte", false, "try to repair broken blobs by trying bytes")
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
	return data.ForAllSnapshots(ctx, repo, repo, nil, func(id restic.ID, snapshot *data.Snapshot, err error) error {
		if err != nil {
			return err
		}

		if _, err := fmt.Fprintf(wr, "snapshot_id: %v\n", id); err != nil {
			return err
		}

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

func printPacks(ctx context.Context, repo *repository.Repository, wr io.Writer, printer progress.Printer) error {

	var m sync.Mutex
	return restic.ParallelList(ctx, repo, restic.PackFile, repo.Connections(), func(ctx context.Context, id restic.ID, size int64) error {
		blobs, err := repo.ListPack(ctx, id, size)
		if err != nil {
			printer.E("error for pack %v: %v", id.Str(), err)
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

		m.Lock()
		defer m.Unlock()
		return prettyPrintJSON(wr, p)
	})
}

func runDebugDump(ctx context.Context, gopts global.Options, args []string, term ui.Terminal) error {
	printer := ui.NewProgressPrinter(false, gopts.Verbosity, term)

	if len(args) != 1 {
		return errors.Fatal("type not specified")
	}

	ctx, repo, unlock, err := openWithReadLock(ctx, gopts, gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	tpe := args[0]

	switch tpe {
	case "indexes":
		return repository.DumpIndexes(ctx, repo, gopts.Term.OutputWriter(), printer)
	case "snapshots":
		return debugPrintSnapshots(ctx, repo, gopts.Term.OutputWriter())
	case "packs":
		return printPacks(ctx, repo, gopts.Term.OutputWriter(), printer)
	case "all":
		printer.S("snapshots:")
		err := debugPrintSnapshots(ctx, repo, gopts.Term.OutputWriter())
		if err != nil {
			return err
		}

		printer.S("indexes:")
		err = repository.DumpIndexes(ctx, repo, gopts.Term.OutputWriter(), printer)
		if err != nil {
			return err
		}

		return nil
	default:
		return errors.Fatalf("no such type %q", tpe)
	}
}

func runDebugExamine(ctx context.Context, gopts global.Options, opts DebugExamineOptions, args []string, term ui.Terminal) error {
	printer := ui.NewProgressPrinter(false, gopts.Verbosity, term)

	if opts.ExtractPack && gopts.NoLock {
		return fmt.Errorf("--extract-pack and --no-lock are mutually exclusive")
	}

	ctx, repo, unlock, err := openWithAppendLock(ctx, gopts, gopts.NoLock, printer)
	if err != nil {
		return err
	}
	defer unlock()

	ids := make([]restic.ID, 0)
	for _, name := range args {
		id, err := restic.ParseID(name)
		if err != nil {
			id, err = restic.Find(ctx, repo, restic.PackFile, name)
			if err != nil {
				printer.E("error: %v", err)
				continue
			}
		}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return errors.Fatal("no pack files to examine")
	}

	err = repo.LoadIndex(ctx, printer)
	if err != nil {
		return err
	}

	examineOpts := repository.ExaminePackOptions{
		TryRepair:     opts.TryRepair,
		RepairByte:    opts.RepairByte,
		ExtractPack:   opts.ExtractPack,
		ReuploadBlobs: opts.ReuploadBlobs,
	}
	for _, id := range ids {
		err := repository.ExaminePack(ctx, repo, id, examineOpts, printer)
		if err != nil {
			printer.E("error: %v", err)
		}
		if err == context.Canceled {
			break
		}
	}
	return nil
}
