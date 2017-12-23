package restic

import (
	"context"
	"fmt"
	"os/user"
	"path/filepath"
	"time"

	"github.com/restic/restic/internal/debug"
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
func NewSnapshot(paths []string, tags []string, hostname string, time time.Time) (*Snapshot, error) {
	for i, path := range paths {
		if p, err := filepath.Abs(path); err == nil {
			paths[i] = p
		}
	}

	sn := &Snapshot{
		Paths:    paths,
		Time:     time,
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
func LoadSnapshot(ctx context.Context, repo Repository, id ID) (*Snapshot, error) {
	sn := &Snapshot{id: &id}
	err := repo.LoadJSONUnpacked(ctx, SnapshotFile, id, sn)
	if err != nil {
		return nil, err
	}

	return sn, nil
}

// LoadAllSnapshots returns a list of all snapshots in the repo.
func LoadAllSnapshots(ctx context.Context, repo Repository) (snapshots []*Snapshot, err error) {
	err = repo.List(ctx, SnapshotFile, func(id ID, size int64) error {
		sn, err := LoadSnapshot(ctx, repo, id)
		if err != nil {
			return err
		}

		snapshots = append(snapshots, sn)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return snapshots, nil
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

func (sn *Snapshot) hasTag(tag string) bool {
	for _, snTag := range sn.Tags {
		if tag == snTag {
			return true
		}
	}
	return false
}

// HasTags returns true if the snapshot has all the tags in l.
func (sn *Snapshot) HasTags(l []string) bool {
	for _, tag := range l {
		if !sn.hasTag(tag) {
			return false
		}
	}

	return true
}

// HasTagList returns true if the snapshot satisfies at least one TagList,
// so there is a TagList in l for which all tags are included in sn.
func (sn *Snapshot) HasTagList(l []TagList) bool {
	debug.Log("testing snapshot with tags %v against list: %v", sn.Tags, l)

	if len(l) == 0 {
		return true
	}

	for _, tags := range l {
		if sn.HasTags(tags) {
			debug.Log("  snapshot satisfies %v %v", tags, l)
			return true
		}
	}

	return false
}

func (sn *Snapshot) hasPath(path string) bool {
	for _, snPath := range sn.Paths {
		if path == snPath {
			return true
		}
	}
	return false
}

// HasPaths returns true if the snapshot has all of the paths.
func (sn *Snapshot) HasPaths(paths []string) bool {
	for _, path := range paths {
		if !sn.hasPath(path) {
			return false
		}
	}

	return true
}

// Snapshots is a list of snapshots.
type Snapshots []*Snapshot

// Len returns the number of snapshots in sn.
func (sn Snapshots) Len() int {
	return len(sn)
}

// Less returns true iff the ith snapshot has been made after the jth.
func (sn Snapshots) Less(i, j int) bool {
	return sn[i].Time.After(sn[j].Time)
}

// Swap exchanges the two snapshots.
func (sn Snapshots) Swap(i, j int) {
	sn[i], sn[j] = sn[j], sn[i]
}
