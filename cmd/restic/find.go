package main

import (
	"context"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/restic"
	"github.com/spf13/pflag"
)

type snapshotFilterOptions struct {
	Hosts []string
	Tags  restic.TagLists
	Paths []string
}

// initMultiSnapshotFilterOptions is used for commands that work on multiple snapshots
func initMultiSnapshotFilterOptions(flags *pflag.FlagSet, options *snapshotFilterOptions, addHostShorthand bool) {
	hostShorthand := "H"
	if !addHostShorthand {
		hostShorthand = ""
	}
	flags.StringArrayVarP(&options.Hosts, "host", hostShorthand, nil, "only consider snapshots for this `host` (can be specified multiple times)")
	flags.Var(&options.Tags, "tag", "only consider snapshots including `tag[,tag,...]` (can be specified multiple times)")
	flags.StringArrayVar(&options.Paths, "path", nil, "only consider snapshots including this (absolute) `path` (can be specified multiple times)")
}

// initSingleSnapshotFilterOptions is used for commands that work on a single snapshot
func initSingleSnapshotFilterOptions(flags *pflag.FlagSet, options *snapshotFilterOptions) {
	flags.StringArrayVarP(&options.Hosts, "host", "H", nil, "only consider snapshots for this `host`, when snapshot ID \"latest\" is given (can be specified multiple times)")
	flags.Var(&options.Tags, "tag", "only consider snapshots including `tag[,tag,...]`, when snapshot ID \"latest\" is given (can be specified multiple times)")
	flags.StringArrayVar(&options.Paths, "path", nil, "only consider snapshots including this (absolute) `path`, when snapshot ID \"latest\" is given (can be specified multiple times)")
}

// FindFilteredSnapshots yields Snapshots, either given explicitly by `snapshotIDs` or filtered from the list of all snapshots.
func FindFilteredSnapshots(ctx context.Context, be restic.Lister, loader restic.LoaderUnpacked, hosts []string, tags []restic.TagList, paths []string, snapshotIDs []string) <-chan *restic.Snapshot {
	out := make(chan *restic.Snapshot)
	go func() {
		defer close(out)
		if len(snapshotIDs) != 0 {
			// memorize snapshots list to prevent repeated backend listings
			be, err := backend.MemorizeList(ctx, be, restic.SnapshotFile)
			if err != nil {
				Warnf("could not load snapshots: %v\n", err)
				return
			}

			var (
				id         restic.ID
				usedFilter bool
			)
			ids := make(restic.IDs, 0, len(snapshotIDs))
			// Process all snapshot IDs given as arguments.
			for _, s := range snapshotIDs {
				if s == "latest" {
					usedFilter = true
					id, err = restic.FindLatestSnapshot(ctx, be, loader, paths, tags, hosts, nil)
					if err != nil {
						Warnf("Ignoring %q, no snapshot matched given filter (Paths:%v Tags:%v Hosts:%v)\n", s, paths, tags, hosts)
						continue
					}
				} else {
					id, err = restic.FindSnapshot(ctx, be, s)
					if err != nil {
						Warnf("Ignoring %q: %v\n", s, err)
						continue
					}
				}
				ids = append(ids, id)
			}

			// Give the user some indication their filters are not used.
			if !usedFilter && (len(hosts) != 0 || len(tags) != 0 || len(paths) != 0) {
				Warnf("Ignoring filters as there are explicit snapshot ids given\n")
			}

			for _, id := range ids.Uniq() {
				sn, err := restic.LoadSnapshot(ctx, loader, id)
				if err != nil {
					Warnf("Ignoring %q, could not load snapshot: %v\n", id, err)
					continue
				}
				select {
				case <-ctx.Done():
					return
				case out <- sn:
				}
			}
			return
		}

		snapshots, err := restic.FindFilteredSnapshots(ctx, be, loader, hosts, tags, paths)
		if err != nil {
			Warnf("could not load snapshots: %v\n", err)
			return
		}

		for _, sn := range snapshots {
			select {
			case <-ctx.Done():
				return
			case out <- sn:
			}
		}
	}()
	return out
}
