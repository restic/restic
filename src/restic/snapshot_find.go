package restic

import (
	"context"
	"restic/errors"
	"time"
)

// ErrNoSnapshotFound is returned when no snapshot for the given criteria could be found.
var ErrNoSnapshotFound = errors.New("no snapshot found")

// FindLatestSnapshot finds latest snapshot with optional target/directory, tags and hostname filters.
func FindLatestSnapshot(ctx context.Context, repo Repository, targets []string, tags []string, hostname string) (ID, error) {
	var (
		latest   time.Time
		latestID ID
		found    bool
	)

	for snapshotID := range repo.List(ctx, SnapshotFile) {
		snapshot, err := LoadSnapshot(ctx, repo, snapshotID)
		if err != nil {
			return ID{}, errors.Errorf("Error listing snapshot: %v", err)
		}
		if snapshot.Time.After(latest) && (hostname == "" || hostname == snapshot.Hostname) && snapshot.HasTags(tags) && snapshot.HasPaths(targets) {
			latest = snapshot.Time
			latestID = snapshotID
			found = true
		}
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
