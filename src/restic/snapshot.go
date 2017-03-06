package restic

import (
	"fmt"
	"os/user"
	"path/filepath"
	"time"

	"restic/errors"
)

// Snapshot is the state of a resource at one point in time.
type Snapshot struct {
	Time     time.Time `json:"time"`
	Parent   *ID       `json:"parent,omitempty"`
	Tree     *ID       `json:"tree"`
	Paths    []string  `json:"paths"`
	Hostname string    `json:"hostname,omitempty"`
	Username string    `json:"username,omitempty"`
	UID      uint32    `json:"uid,omitempty"`
	GID      uint32    `json:"gid,omitempty"`
	Excludes []string  `json:"excludes,omitempty"`
	Tags     []string  `json:"tags,omitempty"`
	Original *ID       `json:"original,omitempty"`

	id *ID // plaintext ID, used during restore
}

// NewSnapshot returns an initialized snapshot struct for the current user and
// time.
func NewSnapshot(paths []string, tags []string, hostname string) (*Snapshot, error) {
	for i, path := range paths {
		if p, err := filepath.Abs(path); err != nil {
			paths[i] = p
		}
	}

	sn := &Snapshot{
		Paths:    paths,
		Time:     time.Now(),
		Tags:     tags,
		Hostname: hostname,
	}

	err := sn.fillUserInfo()
	if err != nil {
		return nil, err
	}

	return sn, nil
}

// LoadSnapshot loads the snapshot with the id and returns it.
func LoadSnapshot(repo Repository, id ID) (*Snapshot, error) {
	sn := &Snapshot{id: &id}
	err := repo.LoadJSONUnpacked(SnapshotFile, id, sn)
	if err != nil {
		return nil, err
	}

	return sn, nil
}

// LoadAllSnapshots returns a list of all snapshots in the repo.
func LoadAllSnapshots(repo Repository) (snapshots []*Snapshot, err error) {
	done := make(chan struct{})
	defer close(done)

	for id := range repo.List(SnapshotFile, done) {
		sn, err := LoadSnapshot(repo, id)
		if err != nil {
			return nil, err
		}

		snapshots = append(snapshots, sn)
	}
	return
}

func (sn Snapshot) String() string {
	return fmt.Sprintf("<Snapshot %s of %v at %s by %s@%s>",
		sn.id.Str(), sn.Paths, sn.Time, sn.Username, sn.Hostname)
}

// ID returns the snapshot's ID.
func (sn Snapshot) ID() *ID {
	return sn.id
}

func (sn *Snapshot) fillUserInfo() error {
	usr, err := user.Current()
	if err != nil {
		return nil
	}
	sn.Username = usr.Username

	// set userid and groupid
	sn.UID, sn.GID, err = uidGidInt(*usr)
	return err
}

// AddTags adds the given tags to the snapshots tags, preventing duplicates.
// It returns true if any changes were made.
func (sn *Snapshot) AddTags(addTags []string) (changed bool) {
nextTag:
	for _, add := range addTags {
		for _, tag := range sn.Tags {
			if tag == add {
				continue nextTag
			}
		}
		sn.Tags = append(sn.Tags, add)
		changed = true
	}
	return
}

// RemoveTags removes the given tags from the snapshots tags and
// returns true if any changes were made.
func (sn *Snapshot) RemoveTags(removeTags []string) (changed bool) {
	for _, remove := range removeTags {
		for i, tag := range sn.Tags {
			if tag == remove {
				// https://github.com/golang/go/wiki/SliceTricks
				sn.Tags[i] = sn.Tags[len(sn.Tags)-1]
				sn.Tags[len(sn.Tags)-1] = ""
				sn.Tags = sn.Tags[:len(sn.Tags)-1]

				changed = true
				break
			}
		}
	}
	return
}

// HasTags returns true if the snapshot has at least all of tags.
func (sn *Snapshot) HasTags(tags []string) bool {
nextTag:
	for _, tag := range tags {
		for _, snTag := range sn.Tags {
			if tag == snTag {
				continue nextTag
			}
		}

		return false
	}

	return true
}

// HasPaths returns true if the snapshot has at least all of paths.
func (sn *Snapshot) HasPaths(paths []string) bool {
nextPath:
	for _, path := range paths {
		for _, snPath := range sn.Paths {
			if path == snPath {
				continue nextPath
			}
		}

		return false
	}

	return true
}

// SamePaths returns true if the snapshot matches the entire paths set
func (sn *Snapshot) SamePaths(paths []string) bool {
	if len(sn.Paths) != len(paths) {
		return false
	}
	return sn.HasPaths(paths)
}

// ErrNoSnapshotFound is returned when no snapshot for the given criteria could be found.
var ErrNoSnapshotFound = errors.New("no snapshot found")

// FindLatestSnapshot finds latest snapshot with optional target/directory and hostname filters.
func FindLatestSnapshot(repo Repository, targets []string, hostname string) (ID, error) {
	var (
		latest   time.Time
		latestID ID
		found    bool
	)

	for snapshotID := range repo.List(SnapshotFile, make(chan struct{})) {
		snapshot, err := LoadSnapshot(repo, snapshotID)
		if err != nil {
			return ID{}, errors.Errorf("Error listing snapshot: %v", err)
		}
		if snapshot.Time.After(latest) && snapshot.HasPaths(targets) && (hostname == "" || hostname == snapshot.Hostname) {
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
