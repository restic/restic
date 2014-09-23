package khepri

import (
	"os"
	"path/filepath"

	"github.com/fd0/khepri/backend"
)

type Archiver struct {
	be   backend.Server
	key  *Key
	ch   *ContentHandler
	smap *StorageMap // blobs used for the current snapshot

	Error  func(dir string, fi os.FileInfo, err error) error
	Filter func(item string, fi os.FileInfo) bool
}

func NewArchiver(be backend.Server, key *Key) (*Archiver, error) {
	var err error
	arch := &Archiver{be: be, key: key}

	// abort on all errors
	arch.Error = func(string, os.FileInfo, error) error { return err }
	// allow all files
	arch.Filter = func(string, os.FileInfo) bool { return true }

	arch.smap = NewStorageMap()
	arch.ch, err = NewContentHandler(be, key)
	if err != nil {
		return nil, err
	}

	// load all blobs from all snapshots
	err = arch.ch.LoadAllSnapshots()
	if err != nil {
		return nil, err
	}

	return arch, nil
}

func (arch *Archiver) Save(t backend.Type, data []byte) (*Blob, error) {
	blob, err := arch.ch.Save(t, data)
	if err != nil {
		return nil, err
	}

	// store blob in storage map for current snapshot
	arch.smap.Insert(blob)

	return blob, nil
}

func (arch *Archiver) SaveJSON(t backend.Type, item interface{}) (*Blob, error) {
	blob, err := arch.ch.SaveJSON(t, item)
	if err != nil {
		return nil, err
	}

	// store blob in storage map for current snapshot
	arch.smap.Insert(blob)

	return blob, nil
}

func (arch *Archiver) SaveFile(node *Node) (Blobs, error) {
	blobs, err := arch.ch.SaveFile(node.path, uint(node.Size))
	if err != nil {
		return nil, arch.Error(node.path, nil, err)
	}

	node.Content = make([]backend.ID, len(blobs))
	for i, blob := range blobs {
		node.Content[i] = blob.ID
		arch.smap.Insert(blob)
	}

	return blobs, err
}

func (arch *Archiver) ImportDir(dir string) (Tree, error) {
	fd, err := os.Open(dir)
	defer fd.Close()
	if err != nil {
		return nil, arch.Error(dir, nil, err)
	}

	entries, err := fd.Readdir(-1)
	if err != nil {
		return nil, arch.Error(dir, nil, err)
	}

	if len(entries) == 0 {
		return nil, nil
	}

	tree := Tree{}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())

		if !arch.Filter(path, entry) {
			continue
		}

		node, err := NodeFromFileInfo(path, entry)
		if err != nil {
			return nil, arch.Error(dir, entry, err)
		}

		tree = append(tree, node)

		if entry.IsDir() {
			subtree, err := arch.ImportDir(path)
			if err != nil {
				return nil, err
			}

			blob, err := arch.SaveJSON(backend.Tree, subtree)
			if err != nil {
				return nil, err
			}

			node.Subtree = blob.ID

			continue
		}

		if node.Type == "file" {
			_, err := arch.SaveFile(node)
			if err != nil {
				return nil, arch.Error(path, entry, err)
			}
		}
	}

	return tree, nil
}

func (arch *Archiver) Import(dir string) (*Snapshot, *Blob, error) {
	sn := NewSnapshot(dir)

	fi, err := os.Lstat(dir)
	if err != nil {
		return nil, nil, err
	}

	node, err := NodeFromFileInfo(dir, fi)
	if err != nil {
		return nil, nil, err
	}

	if node.Type == "dir" {
		tree, err := arch.ImportDir(dir)
		if err != nil {
			return nil, nil, err
		}

		blob, err := arch.SaveJSON(backend.Tree, tree)
		if err != nil {
			return nil, nil, err
		}

		node.Subtree = blob.ID
	} else if node.Type == "file" {
		_, err := arch.SaveFile(node)
		if err != nil {
			return nil, nil, err
		}
	}

	blob, err := arch.SaveJSON(backend.Tree, &Tree{node})
	if err != nil {
		return nil, nil, err
	}

	sn.Content = blob.ID

	// save snapshot
	sn.StorageMap = arch.smap
	blob, err = arch.SaveJSON(backend.Snapshot, sn)
	if err != nil {
		return nil, nil, err
	}

	return sn, blob, nil
}
