package restic

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
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
	UID      uint32     `json:"uid,omitempty"`
	GID      uint32     `json:"gid,omitempty"`

	id backend.ID // plaintext ID, used during restore
	bl *BlobList
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

	usr, err := user.Current()
	if err == nil {
		sn.Username = usr.Username
		uid, err := strconv.ParseInt(usr.Uid, 10, 32)
		if err != nil {
			return nil, err
		}
		sn.UID = uint32(uid)

		gid, err := strconv.ParseInt(usr.Gid, 10, 32)
		if err != nil {
			return nil, err
		}
		sn.GID = uint32(gid)
	}

	return sn, nil
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
