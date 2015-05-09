package restic

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"time"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/repository"
)

type Snapshot struct {
	Time     time.Time  `json:"time"`
	Parent   backend.ID `json:"parent,omitempty"`
	Tree     backend.ID `json:"tree"`
	Paths    []string   `json:"paths"`
	Hostname string     `json:"hostname,omitempty"`
	Username string     `json:"username,omitempty"`
	UID      uint32     `json:"uid,omitempty"`
	GID      uint32     `json:"gid,omitempty"`

	id backend.ID // plaintext ID, used during restore
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
	sn := &Snapshot{id: id}
	err := repo.LoadJSONUnpacked(backend.Snapshot, id, sn)
	if err != nil {
		return nil, err
	}

	return sn, nil
}

func (sn Snapshot) String() string {
	return fmt.Sprintf("<Snapshot of %v at %s>", sn.Paths, sn.Time)
}

func (sn Snapshot) ID() backend.ID {
	return sn.id
}

func (sn *Snapshot) fillUserInfo() error {
	usr, err := user.Current()
	if err != nil {
		return err
	}

	sn.Username = usr.Username
	uid, err := strconv.ParseInt(usr.Uid, 10, 32)
	if err != nil {
		return err
	}
	sn.UID = uint32(uid)

	gid, err := strconv.ParseInt(usr.Gid, 10, 32)
	if err != nil {
		return err
	}
	sn.GID = uint32(gid)

	return nil
}
