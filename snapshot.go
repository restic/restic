package restic

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/restic/restic/backend"
)

type Snapshot struct {
	Time     time.Time  `json:"time"`
	Parent   backend.ID `json:"parent,omitempty"`
	Tree     Blob       `json:"tree"`
	Dir      string     `json:"dir"`
	Hostname string     `json:"hostname,omitempty"`
	Username string     `json:"username,omitempty"`
	UID      uint32     `json:"uid,omitempty"`
	GID      uint32     `json:"gid,omitempty"`
	UserID   string     `json:"userid,omitempty"`
	GroupID  string     `json:"groupid,omitempty"`

	id backend.ID // plaintext ID, used during restore
}

func NewSnapshot(dir string) (*Snapshot, error) {
	d, err := filepath.Abs(dir)
	if err != nil {
		d = dir
	}

	sn := &Snapshot{
		Dir:  d,
		Time: time.Now(),
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

func LoadSnapshot(s Server, id backend.ID) (*Snapshot, error) {
	sn := &Snapshot{id: id}
	err := s.LoadJSONID(backend.Snapshot, id, sn)
	if err != nil {
		return nil, err
	}

	return sn, nil
}

func (sn Snapshot) String() string {
	return fmt.Sprintf("<Snapshot %q at %s>", sn.Dir, sn.Time)
}

func (sn Snapshot) ID() backend.ID {
	return sn.id
}
