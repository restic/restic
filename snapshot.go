package khepri

import (
	"encoding/json"
	"os"
	"os/user"
	"time"
)

type Snapshot struct {
	Time     time.Time `json:"time"`
	Tree     ID        `json:"tree"`
	Dir      string    `json:"dir"`
	Hostname string    `json:"hostname,omitempty"`
	Username string    `json:"username,omitempty"`
	UID      string    `json:"uid,omitempty"`
	GID      string    `json:"gid,omitempty"`
	id       ID
	repo     *Repository
}

func (repo *Repository) NewSnapshot(dir string) *Snapshot {
	sn := &Snapshot{
		Dir:  dir,
		repo: repo,
		Time: time.Now(),
	}

	hn, err := os.Hostname()
	if err == nil {
		sn.Hostname = hn
	}

	usr, err := user.Current()
	if err == nil {
		sn.Username = usr.Username
		sn.UID = usr.Uid
		sn.GID = usr.Gid
	}

	return sn
}

func (sn *Snapshot) Save() error {
	if sn.Tree == nil {
		panic("Snapshot.Save() called with nil tree id")
	}

	obj, err := sn.repo.NewObject(TYPE_REF)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(obj)
	err = enc.Encode(sn)
	if err != nil {
		return err
	}

	err = obj.Close()
	if err != nil {
		return err
	}

	sn.id = obj.ID()
	return nil
}

func (sn *Snapshot) ID() ID {
	return sn.id
}
