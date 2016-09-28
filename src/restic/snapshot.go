package restic

import (
	"fmt"
	"os"
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

	id *ID // plaintext ID, used during restore
}

// NewSnapshot returns an initialized snapshot struct for the current user and
// time.
func NewSnapshot(paths []string, tags []string) (*Snapshot, error) {
	for i, path := range paths {
		if p, err := filepath.Abs(path); err != nil {
			paths[i] = p
		}
	}

	sn := &Snapshot{
		Paths: paths,
		Time:  time.Now(),
		Tags:  tags,
	}

	hn, err := os.Hostname()
	if err == nil {
		sn.Hostname = hn
	}

	err = sn.fillUserInfo()
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

	return snapshots, nil
}

func (sn Snapshot) String() string {
	return fmt.Sprintf("<Snapshot %s of %v at %s by %s@%s>",
		sn.id.Str(), sn.Paths, sn.Time, sn.Username, sn.Hostname)
}

// ID retuns the snapshot's ID.
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

// HasTags returns true if the snapshot has all the tags.
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

// SamePaths compares the Snapshot's paths and provided paths are exactly the same
func SamePaths(expected, actual []string) bool {
	if len(expected) == 0 || len(actual) == 0 {
		return true
	}

	for i := range expected {
		found := false
		for j := range actual {
			if expected[i] == actual[j] {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// ErrNoSnapshotFound is returned when no snapshot for the given criteria could be found.
var ErrNoSnapshotFound = errors.New("no snapshot found")

// FindLatestSnapshot finds latest snapshot with optional target/directory and source filters
func FindLatestSnapshot(repo Repository, targets []string, source string) (ID, error) {
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
		if snapshot.Time.After(latest) && SamePaths(snapshot.Paths, targets) && (source == "" || source == snapshot.Hostname) {
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
