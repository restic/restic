package restic

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/juju/arrar"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/chunker"
)

const (
	maxConcurrentFiles = 8
	maxConcurrentBlobs = 8

	statTimeout = 20 * time.Millisecond
)

type Archiver struct {
	be  backend.Server
	key *Key
	ch  *ContentHandler

	bl       *BlobList // blobs used for the current snapshot
	parentBl *BlobList // blobs from the parent snapshot

	fileToken chan struct{}
	blobToken chan struct{}

	Stats Stats

	Error  func(dir string, fi os.FileInfo, err error) error
	Filter func(item string, fi os.FileInfo) bool

	ScannerStats chan Stats
	SaveStats    chan Stats

	statsMutex  sync.Mutex
	updateStats Stats
}

type Stats struct {
	Files       int
	Directories int
	Other       int
	Bytes       uint64
}

func (s *Stats) Add(other Stats) {
	s.Bytes += other.Bytes
	s.Directories += other.Directories
	s.Files += other.Files
	s.Other += other.Other
}

func NewArchiver(be backend.Server, key *Key) (*Archiver, error) {
	var err error
	arch := &Archiver{
		be:        be,
		key:       key,
		fileToken: make(chan struct{}, maxConcurrentFiles),
		blobToken: make(chan struct{}, maxConcurrentBlobs),
	}

	// fill file and blob token
	for i := 0; i < maxConcurrentFiles; i++ {
		arch.fileToken <- struct{}{}
	}

	for i := 0; i < maxConcurrentBlobs; i++ {
		arch.blobToken <- struct{}{}
	}

	// abort on all errors
	arch.Error = func(string, os.FileInfo, error) error { return err }
	// allow all files
	arch.Filter = func(string, os.FileInfo) bool { return true }

	arch.bl = NewBlobList()
	arch.ch, err = NewContentHandler(be, key)
	if err != nil {
		return nil, err
	}

	// load all blobs from all snapshots
	err = arch.ch.LoadAllMaps()
	if err != nil {
		return nil, err
	}

	return arch, nil
}

func (arch *Archiver) update(ch chan Stats, stats Stats) {
	if ch == nil {
		return
	}

	// load old stats from global state
	arch.statsMutex.Lock()
	stats.Add(arch.updateStats)
	arch.updateStats = Stats{}
	arch.statsMutex.Unlock()

	// try to send stats through the channel, with a timeout
	timeout := time.After(statTimeout)

	select {
	case ch <- stats:
		break
	case _ = <-timeout:

		// save cumulated stats to global state
		arch.statsMutex.Lock()
		arch.updateStats.Add(stats)
		arch.statsMutex.Unlock()

		break
	}
}

func (arch *Archiver) Save(t backend.Type, data []byte) (Blob, error) {
	blob, err := arch.ch.Save(t, data)
	if err != nil {
		return Blob{}, err
	}

	// store blob in storage map for current snapshot
	arch.bl.Insert(blob)

	return blob, nil
}

func (arch *Archiver) SaveJSON(t backend.Type, item interface{}) (Blob, error) {
	blob, err := arch.ch.SaveJSON(t, item)
	if err != nil {
		return Blob{}, err
	}

	// store blob in storage map for current snapshot
	arch.bl.Insert(blob)

	return blob, nil
}

// SaveFile stores the content of the file on the backend as a Blob by calling
// Save for each chunk.
func (arch *Archiver) SaveFile(node *Node) error {
	file, err := os.Open(node.path)
	defer file.Close()
	if err != nil {
		return arrar.Annotate(err, "SaveFile()")
	}

	var blobs Blobs

	// if the file is small enough, store it directly
	if node.Size < chunker.MinSize {
		// acquire token
		token := <-arch.blobToken
		defer func() {
			arch.blobToken <- token
		}()

		buf := GetChunkBuf("blob single file")
		defer FreeChunkBuf("blob single file", buf)
		n, err := io.ReadFull(file, buf)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return arrar.Annotate(err, "SaveFile() read small file")
		}

		if err == io.EOF {
			// use empty blob list for empty files
			blobs = Blobs{}
		} else {
			blob, err := arch.ch.Save(backend.Data, buf[:n])
			if err != nil {
				return arrar.Annotate(err, "SaveFile() save chunk")
			}

			arch.update(arch.SaveStats, Stats{Bytes: blob.Size})

			blobs = Blobs{blob}
		}
	} else {
		// else store all chunks
		chnker := chunker.New(file)
		chans := [](<-chan Blob){}
		defer chnker.Free()

		chunks := 0

		for {
			buf := GetChunkBuf("blob chunker")
			chunk, err := chnker.Next(buf)
			if err == io.EOF {
				FreeChunkBuf("blob chunker", buf)
				break
			}

			if err != nil {
				FreeChunkBuf("blob chunker", buf)
				return arrar.Annotate(err, "SaveFile() chunker.Next()")
			}

			chunks++

			// acquire token, start goroutine to save chunk
			token := <-arch.blobToken
			resCh := make(chan Blob, 1)

			go func(ch chan<- Blob) {
				blob, err := arch.ch.Save(backend.Data, chunk.Data)
				// TODO handle error
				if err != nil {
					panic(err)
				}

				FreeChunkBuf("blob chunker", buf)

				arch.update(arch.SaveStats, Stats{Bytes: blob.Size})
				arch.blobToken <- token
				ch <- blob
			}(resCh)

			chans = append(chans, resCh)
		}

		blobs = []Blob{}
		for _, ch := range chans {
			blobs = append(blobs, <-ch)
		}

		if len(blobs) != chunks {
			return fmt.Errorf("chunker returned %v chunks, but only %v blobs saved", chunks, len(blobs))
		}
	}

	var bytes uint64

	node.Content = make([]backend.ID, len(blobs))
	for i, blob := range blobs {
		node.Content[i] = blob.ID
		arch.bl.Insert(blob)
		bytes += blob.Size
	}

	if bytes != node.Size {
		return fmt.Errorf("errors saving node %q: saved %d bytes, wanted %d bytes", node.path, bytes, node.Size)
	}

	return nil
}

func (arch *Archiver) populateFromOldTree(tree, oldTree Tree) error {
	// update content from old tree
	err := tree.PopulateFrom(oldTree)
	if err != nil {
		return err
	}

	// add blobs to bloblist
	for _, node := range tree {
		if node.Content != nil {
			for _, blobID := range node.Content {
				blob, err := arch.parentBl.Find(Blob{ID: blobID})
				if err != nil {
					return err
				}

				arch.bl.Insert(blob)
			}
		}
	}

	return nil
}

func (arch *Archiver) loadTree(dir string, oldTreeID backend.ID) (*Tree, error) {
	var (
		oldTree Tree
		err     error
	)

	if oldTreeID != nil {
		// load old tree
		oldTree, err = LoadTree(arch.ch, oldTreeID)
		if err != nil {
			return nil, arrar.Annotate(err, "load old tree")
		}

		debug("old tree: %v\n", oldTree)
	}

	// open and list path
	fd, err := os.Open(dir)
	defer fd.Close()
	if err != nil {
		return nil, arch.Error(dir, nil, err)
	}

	entries, err := fd.Readdir(-1)
	if err != nil {
		return nil, err
	}

	// build new tree
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

		err = tree.Insert(node)
		if err != nil {
			return nil, err
		}

		if entry.IsDir() {
			oldSubtree, err := oldTree.Find(node.Name)
			if err != nil && err != ErrNodeNotFound {
				return nil, err
			}

			var oldSubtreeID backend.ID
			if err == nil {
				oldSubtreeID = oldSubtree.Subtree
			}

			node.Tree, err = arch.loadTree(path, oldSubtreeID)
			if err != nil {
				return nil, err
			}
		}
	}

	// populate with content from oldTree
	err = arch.populateFromOldTree(tree, oldTree)
	if err != nil {
		return nil, err
	}

	for _, node := range tree {
		if node.Type == "file" && node.Content != nil {
			continue
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

	arch.update(arch.ScannerStats, arch.Stats)

	return &tree, nil
}

func (arch *Archiver) LoadTree(path string, parentSnapshot backend.ID) (*Tree, error) {
	var oldTree Tree

	if parentSnapshot != nil {
		// load old tree from snapshot
		snapshot, err := LoadSnapshot(arch.ch, parentSnapshot)
		if err != nil {
			return nil, arrar.Annotate(err, "load old snapshot")
		}

		if snapshot.Tree == nil {
			return nil, errors.New("snapshot without tree!")
		}

		// load old bloblist from snapshot
		arch.parentBl, err = LoadBlobList(arch.ch, snapshot.Map)
		if err != nil {
			return nil, err
		}

		oldTree, err = LoadTree(arch.ch, snapshot.Tree)
		if err != nil {
			return nil, arrar.Annotate(err, "load old tree")
		}

		debug("old tree: %v\n", oldTree)
	}

	// reset global stats
	arch.updateStats = Stats{}

	fi, err := os.Lstat(path)
	if err != nil {
		return nil, arrar.Annotatef(err, "Lstat(%q)", path)
	}

	node, err := NodeFromFileInfo(path, fi)
	if err != nil {
		return nil, arrar.Annotate(err, "NodeFromFileInfo()")
	}

	if node.Type != "dir" {
		t := &Tree{node}

		// populate with content from oldTree
		err = arch.populateFromOldTree(*t, oldTree)
		if err != nil {
			return nil, err
		}

		// if no old node has been found, update stats
		if node.Content == nil && node.Subtree == nil {
			arch.Stats.Files = 1
			arch.Stats.Bytes = node.Size
		}

		arch.update(arch.ScannerStats, arch.Stats)

		return t, nil
	}

	arch.Stats.Directories = 1

	var oldSubtreeID backend.ID
	oldSubtree, err := oldTree.Find(node.Name)
	if err != nil && err != ErrNodeNotFound {
		return nil, arrar.Annotate(err, "search node in old tree")
	}

	if err == nil {
		oldSubtreeID = oldSubtree.Subtree
	}

	node.Tree, err = arch.loadTree(path, oldSubtreeID)
	if err != nil {
		return nil, arrar.Annotate(err, "loadTree()")
	}

	arch.update(arch.ScannerStats, arch.Stats)

	return &Tree{node}, nil
}

func (arch *Archiver) saveTree(t *Tree) (Blob, error) {
	var wg sync.WaitGroup

	for _, node := range *t {
		if node.Tree != nil && node.Subtree == nil {
			b, err := arch.saveTree(node.Tree)
			if err != nil {
				return Blob{}, err
			}
			node.Subtree = b.ID
			arch.update(arch.SaveStats, Stats{Directories: 1})
		} else if node.Type == "file" && len(node.Content) == 0 {
			// get token
			token := <-arch.fileToken

			// start goroutine
			wg.Add(1)
			go func(n *Node) {
				defer wg.Done()
				defer func() {
					arch.fileToken <- token
				}()

				// TODO: handle error
				err := arch.SaveFile(n)
				if err != nil {
					panic(err)
				}
				arch.update(arch.SaveStats, Stats{Files: 1})
			}(node)
		} else {
			arch.update(arch.SaveStats, Stats{Other: 1})
		}
	}

	wg.Wait()

	// check for invalid file nodes
	for _, node := range *t {
		if node.Type == "file" && node.Content == nil {
			return Blob{}, fmt.Errorf("node %v has empty content", node.Name)
		}
	}

	blob, err := arch.SaveJSON(backend.Tree, t)
	if err != nil {
		return Blob{}, err
	}

	return blob, nil
}

func (arch *Archiver) Snapshot(dir string, t *Tree, parentSnapshot backend.ID) (*Snapshot, backend.ID, error) {
	// reset global stats
	arch.updateStats = Stats{}

	sn, err := NewSnapshot(dir)
	if err != nil {
		return nil, nil, err
	}

	sn.Parent = parentSnapshot

	blob, err := arch.saveTree(t)
	if err != nil {
		return nil, nil, err
	}
	sn.Tree = blob.ID

	// save bloblist
	blob, err = arch.SaveJSON(backend.Map, arch.bl)
	if err != nil {
		return nil, nil, err
	}
	sn.Map = blob.Storage

	// save snapshot
	blob, err = arch.SaveJSON(backend.Snapshot, sn)
	if err != nil {
		return nil, nil, err
	}

	return sn, blob.Storage, nil
}
