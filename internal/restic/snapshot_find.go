package restic

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/restic/restic/internal/errors"
)

// ErrNoSnapshotFound is returned when no snapshot for the given criteria could be found.
var ErrNoSnapshotFound = errors.New("no snapshot found")

// findLatestSnapshot finds latest snapshot with optional target/directory, tags, hostname, and timestamp filters.
func findLatestSnapshot(ctx context.Context, be Lister, loader LoaderUnpacked, hosts []string,
	tags []TagList, paths []string, timeStampLimit *time.Time) (*Snapshot, error) {

	var err error
	absTargets := make([]string, 0, len(paths))
	for _, target := range paths {
		if !filepath.IsAbs(target) {
			target, err = filepath.Abs(target)
			if err != nil {
				return nil, errors.Wrap(err, "Abs")
			}
		}
		absTargets = append(absTargets, filepath.Clean(target))
	}

	var latest *Snapshot

	err = ForAllSnapshots(ctx, be, loader, nil, func(id ID, snapshot *Snapshot, err error) error {
		if err != nil {
			return errors.Errorf("Error loading snapshot %v: %v", id.Str(), err)
		}

		if timeStampLimit != nil && snapshot.Time.After(*timeStampLimit) {
			return nil
		}

		if latest != nil && snapshot.Time.Before(latest.Time) {
			return nil
		}

		if !snapshot.HasHostname(hosts) {
			return nil
		}

		if !snapshot.HasTagList(tags) {
			return nil
		}

		if !snapshot.HasPaths(absTargets) {
			return nil
		}

		latest = snapshot
		return nil
	})

	if err != nil {
		return nil, err
	}

	if latest == nil {
		return nil, ErrNoSnapshotFound
	}

	return latest, nil
}

// FindSnapshot takes a string and tries to find a snapshot whose ID matches
// the string as closely as possible.
func FindSnapshot(ctx context.Context, be Lister, loader LoaderUnpacked, s string) (*Snapshot, error) {
	// no need to list snapshots if `s` is already a full id
	id, err := ParseID(s)
	if err != nil {
		// find snapshot id with prefix
		id, err = Find(ctx, be, SnapshotFile, s)
		if err != nil {
			return nil, err
		}
	}
	return LoadSnapshot(ctx, loader, id)
}

// FindFilteredSnapshot returns either the latests from a filtered list of all snapshots or a snapshot specified by `snapshotID`.
func FindFilteredSnapshot(ctx context.Context, be Lister, loader LoaderUnpacked, hosts []string, tags []TagList, paths []string, timeStampLimit *time.Time, snapshotID string) (*Snapshot, error) {
	if snapshotID == "latest" {
		sn, err := findLatestSnapshot(ctx, be, loader, hosts, tags, paths, timeStampLimit)
		if err == ErrNoSnapshotFound {
			err = fmt.Errorf("snapshot filter (Paths:%v Tags:%v Hosts:%v): %w", paths, tags, hosts, err)
		}
		return sn, err
	}
	return FindSnapshot(ctx, be, loader, snapshotID)
}

type SnapshotFindCb func(string, *Snapshot, error) error

// FindFilteredSnapshots yields Snapshots, either given explicitly by `snapshotIDs` or filtered from the list of all snapshots.
func FindFilteredSnapshots(ctx context.Context, be Lister, loader LoaderUnpacked, hosts []string, tags []TagList, paths []string, snapshotIDs []string, fn SnapshotFindCb) error {
	if len(snapshotIDs) != 0 {
		var err error
		usedFilter := false

		ids := NewIDSet()
		// Process all snapshot IDs given as arguments.
		for _, s := range snapshotIDs {
			var sn *Snapshot
			if s == "latest" {
				if usedFilter {
					continue
				}

				usedFilter = true

				sn, err = findLatestSnapshot(ctx, be, loader, hosts, tags, paths, nil)
				if err == ErrNoSnapshotFound {
					err = errors.Errorf("no snapshot matched given filter (Paths:%v Tags:%v Hosts:%v)", paths, tags, hosts)
				}
				if sn != nil {
					ids.Insert(*sn.ID())
				}
			} else {
				sn, err = FindSnapshot(ctx, be, loader, s)
				if err == nil {
					if ids.Has(*sn.ID()) {
						continue
					} else {
						ids.Insert(*sn.ID())
						s = sn.ID().String()
					}
				}
			}
			err = fn(s, sn, err)
			if err != nil {
				return err
			}
		}

		// Give the user some indication their filters are not used.
		if !usedFilter && (len(hosts) != 0 || len(tags) != 0 || len(paths) != 0) {
			return fn("filters", nil, errors.Errorf("explicit snapshot ids are given"))
		}
		return nil
	}

	return ForAllSnapshots(ctx, be, loader, nil, func(id ID, sn *Snapshot, err error) error {
		if err != nil {
			return fn(id.String(), sn, err)
		}

		if !sn.HasHostname(hosts) || !sn.HasTagList(tags) || !sn.HasPaths(paths) {
			return nil
		}

		return fn(id.String(), sn, err)
	})
}
