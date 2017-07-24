package restic

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/restic/restic/internal/errors"
)

// ErrNoSnapshotFound is returned when no snapshot for the given criteria could be found.
var ErrNoSnapshotFound = errors.New("no snapshot found")

// FindLatestSnapshot finds latest snapshot with optional target/directory, tags and hostname filters.
func FindLatestSnapshot(ctx context.Context, repo Repository, targets []string, tagLists []TagList, hostname string) (ID, error) {
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
		if snapshot.Time.Before(latest) || (hostname != "" && hostname != snapshot.Hostname) {
			continue
		}

		if !snapshot.HasTagList(tagLists) {
			continue
		}

		if !snapshot.HasPaths(targets) {
			continue
		}

		latest = snapshot.Time
		latestID = snapshotID
		found = true
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
func FindFilteredSnapshots(ctx context.Context, repo Repository, host string, tags []TagList, paths []string) Snapshots {
	results := make(Snapshots, 0, 20)

	for id := range repo.List(ctx, SnapshotFile) {
		sn, err := LoadSnapshot(ctx, repo, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not load snapshot %v: %v\n", id.Str(), err)
			continue
		}
		if (host != "" && host != sn.Hostname) || !sn.HasTagList(tags) || !sn.HasPaths(paths) {
			continue
		}

		results = append(results, sn)
	}
	return results
}
