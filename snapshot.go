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
}

func NewSnapshot(dir string) *Snapshot {
	sn := &Snapshot{
		Dir:  dir,
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

func (sn *Snapshot) Save(repo *Repository) (ID, error) {
	if sn.Tree == nil {
		panic("Snapshot.Save() called with nil tree id")
	}

	obj, id_ch, err := repo.Create(TYPE_REF)
	if err != nil {
		return nil, err
	}

	enc := json.NewEncoder(obj)
	err = enc.Encode(sn)
	if err != nil {
		return nil, err
	}

	err = obj.Close()
	if err != nil {
		return nil, err
	}

	return <-id_ch, nil
}

func LoadSnapshot(repo *Repository, id ID) (*Snapshot, error) {
	rd, err := repo.Get(TYPE_REF, id)
	if err != nil {
		return nil, err
	}

	dec := json.NewDecoder(rd)
	sn := &Snapshot{}
	err = dec.Decode(sn)

	if err != nil {
		return nil, err
	}

	return sn, nil
}
