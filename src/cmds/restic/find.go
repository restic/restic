package main

import (
	"context"

	"restic"
	"restic/repository"
)

// FindFilteredSnapshots yields Snapshots, either given explicitly by `snapshotIDs` or filtered from the list of all snapshots.
func FindFilteredSnapshots(ctx context.Context, repo *repository.Repository, host string, tags []string, paths []string, snapshotIDs []string) <-chan *restic.Snapshot {
	out := make(chan *restic.Snapshot)
	go func() {
		defer close(out)
		if len(snapshotIDs) != 0 {
			var (
				id         restic.ID
				usedFilter bool
				err        error
			)
			ids := make(restic.IDs, 0, len(snapshotIDs))
			// Process all snapshot IDs given as arguments.
			for _, s := range snapshotIDs {
				if s == "latest" {
					id, err = restic.FindLatestSnapshot(ctx, repo, paths, tags, host)
					if err != nil {
						Warnf("Ignoring %q, no snapshot matched given filter (Paths:%v Tags:%v Host:%v)\n", s, paths, tags, host)
						usedFilter = true
						continue
					}
				} else {
					id, err = restic.FindSnapshot(repo, s)
					if err != nil {
						Warnf("Ignoring %q, it is not a snapshot id\n", s)
						continue
					}
				}
				ids = append(ids, id)
			}

			// Give the user some indication their filters are not used.
			if !usedFilter && (host != "" || len(tags) != 0 || len(paths) != 0) {
				Warnf("Ignoring filters as there are explicit snapshot ids given\n")
			}

			for _, id := range ids.Uniq() {
				sn, err := restic.LoadSnapshot(ctx, repo, id)
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

		for id := range repo.List(ctx, restic.SnapshotFile) {
			sn, err := restic.LoadSnapshot(ctx, repo, id)
			if err != nil {
				Warnf("Ignoring %q, could not load snapshot: %v\n", id, err)
				continue
			}
			if (host != "" && host != sn.Hostname) || !sn.HasTags(tags) || !sn.HasPaths(paths) {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case out <- sn:
			}
		}
	}()
	return out
}
