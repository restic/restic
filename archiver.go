package khepri

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/fd0/khepri/backend"
	"github.com/fd0/khepri/chunker"
)

const (
	maxConcurrentFiles = 32
)

type Archiver struct {
	be  backend.Server
	key *Key
	ch  *ContentHandler

	m    sync.Mutex
	smap *StorageMap // blobs used for the current snapshot

	fileToken chan struct{}

	Stats Stats

	Error  func(dir string, fi os.FileInfo, err error) error
	Filter func(item string, fi os.FileInfo) bool

	ScannerUpdate func(stats Stats)
	SaveUpdate    func(stats Stats)

	sum sync.Mutex // for SaveUpdate
}

type Stats struct {
	Files       int
	Directories int
	Other       int
	Bytes       uint64
}

func NewArchiver(be backend.Server, key *Key) (*Archiver, error) {
	var err error
	arch := &Archiver{
		be:        be,
		key:       key,
		fileToken: make(chan struct{}, maxConcurrentFiles),
	}

	// fill file token
	for i := 0; i < maxConcurrentFiles; i++ {
		arch.fileToken <- struct{}{}
	}

	// abort on all errors
	arch.Error = func(string, os.FileInfo, error) error { return err }
	// allow all files
	arch.Filter = func(string, os.FileInfo) bool { return true }
	// do nothing
	arch.ScannerUpdate = func(Stats) {}

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

func (arch *Archiver) saveUpdate(stats Stats) {
	if arch.SaveUpdate != nil {
		arch.sum.Lock()
		defer arch.sum.Unlock()
		arch.SaveUpdate(stats)
	}
}

func (arch *Archiver) Save(t backend.Type, data []byte) (*Blob, error) {
	blob, err := arch.ch.Save(t, data)
	if err != nil {
		return nil, err
	}

	// store blob in storage map for current snapshot
	arch.m.Lock()
	defer arch.m.Unlock()
	arch.smap.Insert(blob)

	return blob, nil
}

func (arch *Archiver) SaveJSON(t backend.Type, item interface{}) (*Blob, error) {
	blob, err := arch.ch.SaveJSON(t, item)
	if err != nil {
		return nil, err
	}

	// store blob in storage map for current snapshot
	arch.m.Lock()
	defer arch.m.Unlock()
	arch.smap.Insert(blob)

	return blob, nil
}

// SaveFile stores the content of the file on the backend as a Blob by calling
// Save for each chunk.
func (arch *Archiver) SaveFile(node *Node) error {
	file, err := os.Open(node.path)
	defer file.Close()
	if err != nil {
		return err
	}

	var blobs Blobs

	// if the file is small enough, store it directly
	if node.Size < chunker.MinSize {
		buf, err := ioutil.ReadAll(file)
		if err != nil {
			return err
		}

		blob, err := arch.ch.Save(backend.Data, buf)
		if err != nil {
			return err
		}

		arch.saveUpdate(Stats{Bytes: blob.Size})

		blobs = Blobs{blob}
	} else {
		// else store all chunks
		chunker := chunker.New(file)

		for {
			chunk, err := chunker.Next()
			if err == io.EOF {
				break
			}

			if err != nil {
				return err
			}

			blob, err := arch.ch.Save(backend.Data, chunk.Data)
			if err != nil {
				return err
			}

			arch.saveUpdate(Stats{Bytes: blob.Size})

			blobs = append(blobs, blob)
		}
	}

	node.Content = make([]backend.ID, len(blobs))
	for i, blob := range blobs {
		node.Content[i] = blob.ID
		arch.m.Lock()
		arch.smap.Insert(blob)
		arch.m.Unlock()
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
	var wg sync.WaitGroup

	for _, node := range *t {
		if node.Tree != nil && node.Subtree == nil {
			b, err := arch.saveTree(node.Tree)
			if err != nil {
				return nil, err
			}
			node.Subtree = b.ID
			arch.saveUpdate(Stats{Directories: 1})
		} else if node.Type == "file" && len(node.Content) == 0 {
			// start goroutine
			wg.Add(1)
			go func(n *Node) {
				defer wg.Done()

				// get token
				token := <-arch.fileToken
				defer func() {
					arch.fileToken <- token
				}()

				// TODO: handle error
				arch.SaveFile(n)
				arch.saveUpdate(Stats{Files: 1})
			}(node)
		} else {
			arch.saveUpdate(Stats{Other: 1})
		}
	}

	wg.Wait()

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
