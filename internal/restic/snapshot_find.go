package restic

import (
	"context"
	"path/filepath"
	"time"

	"github.com/restic/restic/internal/errors"
)

// ErrNoSnapshotFound is returned when no snapshot for the given criteria could be found.
var ErrNoSnapshotFound = errors.New("no snapshot found")

// FindLatestSnapshot finds latest snapshot with optional target/directory, tags, hostname, and timestamp filters.
func FindLatestSnapshot(ctx context.Context, be Lister, loader LoaderUnpacked, targets []string,
	tagLists []TagList, hostnames []string, timeStampLimit *time.Time) (ID, error) {

	var err error
	absTargets := make([]string, 0, len(targets))
	for _, target := range targets {
		if !filepath.IsAbs(target) {
			target, err = filepath.Abs(target)
			if err != nil {
				return ID{}, errors.Wrap(err, "Abs")
			}
		}
		absTargets = append(absTargets, filepath.Clean(target))
	}

	var (
		latest   time.Time
		latestID ID
		found    bool
	)

	err = ForAllSnapshots(ctx, be, loader, nil, func(id ID, snapshot *Snapshot, err error) error {
		if err != nil {
			return errors.Errorf("Error loading snapshot %v: %v", id.Str(), err)
		}

		if timeStampLimit != nil && snapshot.Time.After(*timeStampLimit) {
			return nil
		}

		if snapshot.Time.Before(latest) {
			return nil
		}

		if !snapshot.HasHostname(hostnames) {
			return nil
		}

		if !snapshot.HasTagList(tagLists) {
			return nil
		}

		if !snapshot.HasPaths(absTargets) {
			return nil
		}

		latest = snapshot.Time
		latestID = id
		found = true
		return nil
	})

	if err != nil {
		return ID{}, err
	}

	if !found {
		return ID{}, ErrNoSnapshotFound
	}

	return latestID, nil
}

// FindSnapshot takes a string and tries to find a snapshot whose ID matches
// the string as closely as possible.
func FindSnapshot(ctx context.Context, be Lister, s string) (ID, error) {

	// find snapshot id with prefix
	name, err := Find(ctx, be, SnapshotFile, s)
	if err != nil {
		return ID{}, err
	}

	return ParseID(name)
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
			var id ID
			if s == "latest" {
				if usedFilter {
					continue
				}

				usedFilter = true

				id, err = FindLatestSnapshot(ctx, be, loader, paths, tags, hosts, nil)
				if err == ErrNoSnapshotFound {
					err = errors.Errorf("no snapshot matched given filter (Paths:%v Tags:%v Hosts:%v)", paths, tags, hosts)
				}
			} else {
				id, err = FindSnapshot(ctx, be, s)
			}

			var sn *Snapshot
			if ids.Has(id) {
				continue
			} else if !id.IsNull() {
				ids.Insert(id)
				sn, err = LoadSnapshot(ctx, loader, id)
				s = id.String()
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
