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

	Stats Stats

	Error  func(dir string, fi os.FileInfo, err error) error
	Filter func(item string, fi os.FileInfo) bool

	ScannerUpdate func(stats Stats)
	SaveUpdate    func(stats Stats)
}

type Stats struct {
	Files       int
	Directories int
	Other       int
	Bytes       uint64
}

func NewArchiver(be backend.Server, key *Key) (*Archiver, error) {
	var err error
	arch := &Archiver{be: be, key: key}

	// abort on all errors
	arch.Error = func(string, os.FileInfo, error) error { return err }
	// allow all files
	arch.Filter = func(string, os.FileInfo) bool { return true }
	// do nothing
	arch.ScannerUpdate = func(Stats) {}
	arch.SaveUpdate = func(Stats) {}

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

func (arch *Archiver) SaveFile(node *Node) error {
	blobs, err := arch.ch.SaveFile(node.path, uint(node.Size))
	if err != nil {
		return arch.Error(node.path, nil, err)
	}

	node.Content = make([]backend.ID, len(blobs))
	for i, blob := range blobs {
		node.Content[i] = blob.ID
		arch.smap.Insert(blob)
	}

	return err
}

func (arch *Archiver) loadTree(dir string) (*Tree, error) {
	// open and list path
	fd, err := os.Open(dir)
	defer fd.Close()
	if err != nil {
		return nil, err
	}

	entries, err := fd.Readdir(-1)
	if err != nil {
		return nil, err
	}

	tree := Tree{}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())

		if !arch.Filter(path, entry) {
			continue
		}

		node, err := NodeFromFileInfo(path, entry)
		if err != nil {
			// TODO: error processing
			return nil, err
		}

		tree = append(tree, node)

		if entry.IsDir() {
			node.Tree, err = arch.loadTree(path)
			if err != nil {
				return nil, err
			}
		}

		switch node.Type {
		case "file":
			arch.Stats.Files++
			arch.Stats.Bytes += node.Size
		case "dir":
			arch.Stats.Directories++
		default:
			arch.Stats.Other++
		}
	}

	arch.ScannerUpdate(arch.Stats)

	return &tree, nil
}

func (arch *Archiver) LoadTree(path string) (*Tree, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}

	node, err := NodeFromFileInfo(path, fi)
	if err != nil {
		return nil, err
	}

	if node.Type != "dir" {
		arch.Stats.Files = 1
		arch.Stats.Bytes = node.Size
		arch.ScannerUpdate(arch.Stats)
		return &Tree{node}, nil
	}

	arch.Stats.Directories = 1
	node.Tree, err = arch.loadTree(path)
	if err != nil {
		return nil, err
	}

	arch.ScannerUpdate(arch.Stats)

	return &Tree{node}, nil
}

func (arch *Archiver) saveTree(t *Tree) (*Blob, error) {
	for _, node := range *t {
		if node.Tree != nil && node.Subtree == nil {
			b, err := arch.saveTree(node.Tree)
			if err != nil {
				return nil, err
			}
			node.Subtree = b.ID
			arch.SaveUpdate(Stats{Directories: 1})
		} else if node.Type == "file" && len(node.Content) == 0 {
			err := arch.SaveFile(node)
			if err != nil {
				return nil, err
			}

			arch.SaveUpdate(Stats{Files: 1, Bytes: node.Size})
		} else {
			arch.SaveUpdate(Stats{Other: 1})
		}
	}

	blob, err := arch.SaveJSON(backend.Tree, t)
	if err != nil {
		return nil, err
	}

	return blob, nil
}

func (arch *Archiver) Snapshot(dir string, t *Tree) (*Snapshot, backend.ID, error) {
	sn := NewSnapshot(dir)

	blob, err := arch.saveTree(t)
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

	return sn, blob.Storage, nil
}
