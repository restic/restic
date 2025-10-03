package main

import (
	"context"
	"os"

	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
	"github.com/spf13/pflag"
)

// initMultiSnapshotFilter is used for commands that work on multiple snapshots
// MUST be combined with restic.FindLatest, restic.FindAll or FindFilteredSnapshots
func initMultiSnapshotFilter(flags *pflag.FlagSet, filt *restic.SnapshotFilter, addHostShorthand bool) {
	hostShorthand := "H"
	if !addHostShorthand {
		hostShorthand = ""
	}
	flags.StringArrayVarP(&filt.Hosts, "host", hostShorthand, nil, "only consider snapshots for this `host` (can be specified multiple times) (default: $RESTIC_HOST)")
	flags.Var(&filt.Tags, "tag", "only consider snapshots including `tag[,tag,...]` (can be specified multiple times)")
	flags.StringArrayVar(&filt.Paths, "path", nil, "only consider snapshots including this (absolute) `path` (can be specified multiple times, snapshots must include all specified paths)")
	flags.Var(&filt.OlderThan, "older-than", "only consider snapshots which are older the snapshot time: use: a duration, a date(time) string or a snapid")
	flags.Var(&filt.NewerThan, "newer-than", "only consider snapshots which are newer the snapshot time: use: a duration, a date(time) string or a snapid")
	flags.Var(&filt.RelativeTo, "relative-to", "define the reference time to which the above durations will refer to: use `now`, a date(time) string, a snapid or `latest`")

	// set default based on env if set
	if host := os.Getenv("RESTIC_HOST"); host != "" {
		filt.Hosts = []string{host}
	}
}

// initSingleSnapshotFilter is used for commands that work on a single snapshot
// MUST be combined with restic.FindLatest
func initSingleSnapshotFilter(flags *pflag.FlagSet, filt *restic.SnapshotFilter) {
	flags.StringArrayVarP(&filt.Hosts, "host", "H", nil, "only consider snapshots for this `host`, when snapshot ID \"latest\" is given (can be specified multiple times) (default: $RESTIC_HOST)")
	flags.Var(&filt.Tags, "tag", "only consider snapshots including `tag[,tag,...]`, when snapshot ID \"latest\" is given (can be specified multiple times)")
	flags.StringArrayVar(&filt.Paths, "path", nil, "only consider snapshots including this (absolute) `path`, when snapshot ID \"latest\" is given (can be specified multiple times, snapshots must include all specified paths)")

	// set default based on env if set
	if host := os.Getenv("RESTIC_HOST"); host != "" {
		filt.Hosts = []string{host}
	}
}

// FindFilteredSnapshots yields Snapshots, either given explicitly by `snapshotIDs` or filtered from the list of all snapshots.
func FindFilteredSnapshots(ctx context.Context, be restic.Lister, loader restic.LoaderUnpacked, f *restic.SnapshotFilter, snapshotIDs []string, printer progress.Printer) <-chan *restic.Snapshot {
	out := make(chan *restic.Snapshot)
	go func() {
		defer close(out)
		be, err := restic.MemorizeList(ctx, be, restic.SnapshotFile)
		if err != nil {
			printer.E("could not load snapshots: %v", err)
			return
		}

		err = f.FindAll(ctx, be, loader, snapshotIDs, func(id string, sn *restic.Snapshot, err error) error {
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
