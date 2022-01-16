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

// abs turns paths to cleaned absolute paths
func abs(targets []string) ([]string, error) {
	var err error
	absTargets := make([]string, 0, len(targets))
	for _, target := range targets {
		if !filepath.IsAbs(target) {
			target, err = filepath.Abs(target)
			if err != nil {
				return nil, errors.Wrap(err, "absTargets")
			}
		}
		absTargets = append(absTargets, filepath.Clean(target))
	}
	return absTargets, nil
}

// FindLatestSnapshot finds latest snapshot with optional target/directory, tags and hostname filters.
func FindLatestSnapshot(ctx context.Context, repo Repository, targets []string, tagLists []TagList, hostnames []string) (ID, error) {
	absTargets, err := abs(targets)
	if err != nil {
		return ID{}, err
	}

	var (
		latest   time.Time
		latestID ID
		found    bool
	)

	err = ForAllSnapshots(ctx, repo, nil, func(id ID, snapshot *Snapshot, err error) error {
		if err != nil {
			return errors.Errorf("Error loading snapshot %v: %v", id.Str(), err)
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

// FindParentSnapshots returns the snapshots that should be picked as parents.
func FindParentSnapshots(ctx context.Context, repo Repository, targets []string, hostname string) (IDs, error) {
	absTargets, err := abs(targets)
	if err != nil {
		return nil, err
	}

	var snapshots []*Snapshot

	err = ForAllSnapshots(ctx, repo, nil, func(snapshotID ID, snapshot *Snapshot, err error) error {
		if err != nil {
			return errors.Errorf("Error loading snapshot %v: %v", snapshotID.Str(), err)
		}

		if !snapshot.HasHostname([]string{hostname}) {
			return nil
		}

		if !snapshot.MatchPaths(absTargets) {
			return nil
		}

		// ignore this snapshot if already superseded
		for _, sn := range snapshots {
			if sn.Supersedes(snapshot, absTargets) {
				fmt.Printf("%v is superseded by %v\n", snapshot.ID(), sn.ID())
				return nil
			}
		}

		// add snapshot after removing snapshots that are superseded by this one
		snapshotsNew := snapshots[:0]
		for _, sn := range snapshots {
			if !snapshot.Supersedes(sn, absTargets) {
				snapshotsNew = append(snapshotsNew, sn)
			} else {
				fmt.Printf("%v is superseded by %v\n", sn.ID(), snapshot.ID())
			}
		}
		snapshots = append(snapshotsNew, snapshot)
		return nil
	})

	if err != nil {
		return nil, err
	}

	snapshotIDs := make(IDs, 0, len(snapshots))
	for _, sn := range snapshots {
		snapshotIDs = append(snapshotIDs, *sn.ID())
	}
	return snapshotIDs, nil
}

// FindSnapshot takes a string and tries to find a snapshot whose ID matches
// the string as closely as possible.
func FindSnapshot(ctx context.Context, repo Repository, s string) (ID, error) {

	// find snapshot id with prefix
	name, err := Find(ctx, repo.Backend(), SnapshotFile, s)
	if err != nil {
		return ID{}, err
	}

	return ParseID(name)
}

// FindFilteredSnapshots yields Snapshots filtered from the list of all
// snapshots.
func FindFilteredSnapshots(ctx context.Context, repo Repository, hosts []string, tags []TagList, paths []string) (Snapshots, error) {
	results := make(Snapshots, 0, 20)

	err := ForAllSnapshots(ctx, repo, nil, func(id ID, sn *Snapshot, err error) error {
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
