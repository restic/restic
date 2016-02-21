package restic

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"restic/backend"
	"restic/repository"
)

type Snapshot struct {
	Time     time.Time   `json:"time"`
	Parent   *backend.ID `json:"parent,omitempty"`
	Tree     *backend.ID `json:"tree"`
	Paths    []string    `json:"paths"`
	Comment  string      `json:"comment"`
	Hostname string      `json:"hostname,omitempty"`
	Username string      `json:"username,omitempty"`
	UID      uint32      `json:"uid,omitempty"`
	GID      uint32      `json:"gid,omitempty"`
	Excludes []string    `json:"excludes,omitempty"`

	id *backend.ID // plaintext ID, used during restore
}

func NewSnapshot(paths []string) (*Snapshot, error) {
	for i, path := range paths {
		if p, err := filepath.Abs(path); err != nil {
			paths[i] = p
		}
	}

	sn := &Snapshot{
		Paths: paths,
		Time:  time.Now(),
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

func LoadSnapshot(repo *repository.Repository, id backend.ID) (*Snapshot, error) {
	sn := &Snapshot{id: &id}
	err := repo.LoadJSONUnpacked(backend.Snapshot, id, sn)
	if err != nil {
		return nil, err
	}

	return sn, nil
}

func (sn Snapshot) String() string {
	return fmt.Sprintf("<Snapshot of %v at %s>", sn.Paths, sn.Time)
}

func (sn Snapshot) ID() *backend.ID {
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

// FindSnapshot takes a string and tries to find a snapshot whose ID matches
// the string as closely as possible.
func FindSnapshot(repo *repository.Repository, s string) (backend.ID, error) {
	// find snapshot id with prefix
	name, err := backend.Find(repo.Backend(), backend.Snapshot, s)
	if err != nil {
		return backend.ID{}, err
	}

	return backend.ParseID(name)
}
