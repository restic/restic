package restic

import (
	"context"
	"fmt"
	"os"
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

// FindFilteredSnapshots yields Snapshots filtered from the list of all
// snapshots.
func FindFilteredSnapshots(ctx context.Context, be Lister, loader LoaderUnpacked, hosts []string, tags []TagList, paths []string) (Snapshots, error) {
	results := make(Snapshots, 0, 20)

	err := ForAllSnapshots(ctx, be, loader, nil, func(id ID, sn *Snapshot, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not load snapshot %v: %v\n", id.Str(), err)
			return nil
		}

		if !sn.HasHostname(hosts) || !sn.HasTagList(tags) || !sn.HasPaths(paths) {
			return nil
		}

		results = append(results, sn)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return results, nil
}
