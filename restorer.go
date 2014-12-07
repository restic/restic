package restic

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/arrar"
	"github.com/restic/restic/backend"
)

type Restorer struct {
	be  backend.Server
	key *Key
	ch  *ContentHandler
	sn  *Snapshot

	Error  func(dir string, node *Node, err error) error
	Filter func(item string, node *Node) bool
}

// NewRestorer creates a restorer preloaded with the content from the snapshot snid.
func NewRestorer(be backend.Server, key *Key, snid backend.ID) (*Restorer, error) {
	r := &Restorer{
		be:  be,
		key: key,
	}

	var err error
	r.ch, err = NewContentHandler(be, key)
	if err != nil {
		return nil, arrar.Annotate(err, "create contenthandler for restorer")
	}

	r.sn, err = r.ch.LoadSnapshot(snid)
	if err != nil {
		return nil, arrar.Annotate(err, "load snapshot for restorer")
	}

	// abort on all errors
	r.Error = func(string, *Node, error) error { return err }
	// allow all files
	r.Filter = func(string, *Node) bool { return true }

	return r, nil
}

func (res *Restorer) to(dir string, tree_id backend.ID) error {
	tree := Tree{}
	err := res.ch.LoadJSON(backend.Tree, tree_id, &tree)
	if err != nil {
		return res.Error(dir, nil, arrar.Annotate(err, "LoadJSON"))
	}

	for _, node := range tree {
		p := filepath.Join(dir, node.Name)
		if !res.Filter(p, node) {
			continue
		}

		err := node.CreateAt(res.ch, p)
		if err != nil {
			err = res.Error(p, node, arrar.Annotate(err, "create node"))
			if err != nil {
				return err
			}
		}

		if node.Type == "dir" {
			if node.Subtree == nil {
				return errors.New(fmt.Sprintf("Dir without subtree in tree %s", tree_id))
			}

			err = res.to(p, node.Subtree)
			if err != nil {
				err = res.Error(p, node, arrar.Annotate(err, "restore subtree"))
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// RestoreTo creates the directories and files in the snapshot below dir.
// Before an item is created, res.Filter is called.
func (res *Restorer) RestoreTo(dir string) error {
	err := os.MkdirAll(dir, 0700)
	if err != nil && err != os.ErrExist {
		return err
	}

	return res.to(dir, res.sn.Tree)
}

func (res *Restorer) Snapshot() *Snapshot {
	return res.sn
}
