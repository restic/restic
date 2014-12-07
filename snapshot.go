package restic

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/restic/restic/backend"
)

type Snapshot struct {
	Time     time.Time  `json:"time"`
	Parent   backend.ID `json:"parent,omitempty"`
	Tree     backend.ID `json:"tree"`
	Map      backend.ID `json:"map"`
	Dir      string     `json:"dir"`
	Hostname string     `json:"hostname,omitempty"`
	Username string     `json:"username,omitempty"`
	UID      string     `json:"uid,omitempty"`
	GID      string     `json:"gid,omitempty"`

	id backend.ID // plaintext ID, used during restore
	bl *BlobList
}

func NewSnapshot(dir string) *Snapshot {
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

	usr, err := user.Current()
	if err == nil {
		sn.Username = usr.Username
		sn.UID = usr.Uid
		sn.GID = usr.Gid
	}

	return sn
}

func LoadSnapshot(ch *ContentHandler, id backend.ID) (*Snapshot, error) {
	sn := &Snapshot{id: id}
	err := ch.LoadJSON(backend.Snapshot, id, sn)
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
