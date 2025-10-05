package main

import (
	"context"
	"os"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/spf13/pflag"
)

// initMultiSnapshotFilter is used for commands that work on multiple snapshots
// MUST be combined with restic.FindFilteredSnapshots or FindFilteredSnapshots
// MUST be followed by finalizeSnapshotFilter after flag parsing
func initMultiSnapshotFilter(flags *pflag.FlagSet, filt *data.SnapshotFilter, addHostShorthand bool) {
	hostShorthand := "H"
	if !addHostShorthand {
		hostShorthand = ""
	}
	flags.StringArrayVarP(&filt.Hosts, "host", hostShorthand, nil, "only consider snapshots for this `host` (can be specified multiple times, use empty string to unset default value) (default: $RESTIC_HOST)")
	flags.Var(&filt.Tags, "tag", "only consider snapshots including `tag[,tag,...]` (can be specified multiple times)")
	flags.StringArrayVar(&filt.Paths, "path", nil, "only consider snapshots including this (absolute) `path` (can be specified multiple times, snapshots must include all specified paths)")
}

// initSingleSnapshotFilter is used for commands that work on a single snapshot
// MUST be combined with restic.FindFilteredSnapshot
// MUST be followed by finalizeSnapshotFilter after flag parsing
func initSingleSnapshotFilter(flags *pflag.FlagSet, filt *data.SnapshotFilter) {
	flags.StringArrayVarP(&filt.Hosts, "host", "H", nil, "only consider snapshots for this `host`, when snapshot ID \"latest\" is given (can be specified multiple times, use empty string to unset default value) (default: $RESTIC_HOST)")
	flags.Var(&filt.Tags, "tag", "only consider snapshots including `tag[,tag,...]`, when snapshot ID \"latest\" is given (can be specified multiple times)")
	flags.StringArrayVar(&filt.Paths, "path", nil, "only consider snapshots including this (absolute) `path`, when snapshot ID \"latest\" is given (can be specified multiple times, snapshots must include all specified paths)")
}

// finalizeSnapshotFilter applies RESTIC_HOST default only if --host flag wasn't explicitly set.
// This allows users to override RESTIC_HOST by providing --host="" or --host with explicit values.
func finalizeSnapshotFilter(filt *data.SnapshotFilter) {
	// Only apply RESTIC_HOST default if the --host flag wasn't changed by the user
	if filt.Hosts == nil {
		if host := os.Getenv("RESTIC_HOST"); host != "" {
			filt.Hosts = []string{host}
		}
	}
	// If flag was set to empty string explicitly (e.g., --host=""),
	// filt.Hosts will be []string{""} which should be cleaned up to allow all hosts
	if len(filt.Hosts) == 1 && filt.Hosts[0] == "" {
		filt.Hosts = nil
	}
}

// FindFilteredSnapshots yields Snapshots, either given explicitly by `snapshotIDs` or filtered from the list of all snapshots.
func FindFilteredSnapshots(ctx context.Context, be restic.Lister, loader restic.LoaderUnpacked, f *data.SnapshotFilter, snapshotIDs []string, printer progress.Printer) <-chan *data.Snapshot {
	out := make(chan *data.Snapshot)
	go func() {
		defer close(out)
		be, err := restic.MemorizeList(ctx, be, restic.SnapshotFile)
		if err != nil {
			printer.E("could not load snapshots: %v", err)
			return
		}

		err = f.FindAll(ctx, be, loader, snapshotIDs, func(id string, sn *data.Snapshot, err error) error {
			if err != nil {
				printer.E("Ignoring %q: %v", id, err)
			} else {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case out <- sn:
				}
			}
			return nil
		})
		if err != nil {
			printer.E("could not load snapshots: %v", err)
		}
	}()
	return out
}
