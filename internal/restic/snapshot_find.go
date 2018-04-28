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

// FindLatestSnapshot finds latest snapshot with optional target/directory, tags and hostname filters.
func FindLatestSnapshot(ctx context.Context, repo Repository, targets []string, tagLists []TagList, hostname string) (ID, error) {
	var err error
	absTargets := make([]string, 0, len(targets))
	for _, target := range targets {
		if !filepath.IsAbs(target) {
			target, err = filepath.Abs(target)
			if err != nil {
				return ID{}, errors.Wrap(err, "Abs")
			}
		}
		absTargets = append(absTargets, target)
	}

	var (
		latest   time.Time
		latestID ID
		found    bool
	)

	err = repo.List(ctx, SnapshotFile, func(snapshotID ID, size int64) error {
		snapshot, err := LoadSnapshot(ctx, repo, snapshotID)
		if err != nil {
			return errors.Errorf("Error loading snapshot %v: %v", snapshotID.Str(), err)
		}
		if snapshot.Time.Before(latest) || (hostname != "" && hostname != snapshot.Hostname) {
			return nil
		}

		if !snapshot.HasTagList(tagLists) {
			return nil
		}

		if !snapshot.HasPaths(absTargets) {
			return nil
		}

		latest = snapshot.Time
		latestID = snapshotID
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
func FindSnapshot(repo Repository, s string) (ID, error) {

	// find snapshot id with prefix
	name, err := Find(repo.Backend(), SnapshotFile, s)
	if err != nil {
		return ID{}, err
	}

	return ParseID(name)
}

// FindFilteredSnapshots yields Snapshots filtered from the list of all
// snapshots.
func FindFilteredSnapshots(ctx context.Context, repo Repository, host string, tags []TagList, paths []string) (Snapshots, error) {
	results := make(Snapshots, 0, 20)

	err := repo.List(ctx, SnapshotFile, func(id ID, size int64) error {
		sn, err := LoadSnapshot(ctx, repo, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not load snapshot %v: %v\n", id.Str(), err)
			return nil
		}

		if (host != "" && host != sn.Hostname) || !sn.HasTagList(tags) || !sn.HasPaths(paths) {
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
