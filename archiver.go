package khepri

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fd0/khepri/backend"
	"github.com/fd0/khepri/chunker"
	"github.com/juju/arrar"
)

const (
	maxConcurrentFiles = 32
	maxConcurrentBlobs = 32

	statTimeout = 20 * time.Millisecond
)

type Archiver struct {
	be  backend.Server
	key *Key
	ch  *ContentHandler

	bl *BlobList // blobs used for the current snapshot

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
	err = arch.ch.LoadAllSnapshots()
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
		buf, err := ioutil.ReadAll(file)
		if err != nil {
			return err
		}

		blob, err := arch.ch.Save(backend.Data, buf)
		if err != nil {
			return err
		}

		arch.update(arch.SaveStats, Stats{Bytes: blob.Size})

		blobs = Blobs{blob}
	} else {
		// else store all chunks
		chnker := chunker.New(file)
		chans := [](<-chan Blob){}

		for {
			chunk, err := chnker.Next()
			if err == io.EOF {
				break
			}

			if err != nil {
				return err
			}

			// acquire token, start goroutine to save chunk
			token := <-arch.blobToken
			resCh := make(chan Blob, 1)

			go func(ch chan<- Blob) {
				blob, err := arch.ch.Save(backend.Data, chunk.Data)
				// TODO handle error
				if err != nil {
					panic(err)
				}

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
	}

	node.Content = make([]backend.ID, len(blobs))
	for i, blob := range blobs {
		node.Content[i] = blob.ID
		arch.bl.Insert(blob)
	}

	return err
}

func (arch *Archiver) loadTree(dir string) (*Tree, error) {
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

	arch.update(arch.ScannerStats, arch.Stats)

	return &tree, nil
}

func (arch *Archiver) LoadTree(path string) (*Tree, error) {
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
		arch.Stats.Files = 1
		arch.Stats.Bytes = node.Size
		arch.update(arch.ScannerStats, arch.Stats)
		return &Tree{node}, nil
	}

	arch.Stats.Directories = 1
	node.Tree, err = arch.loadTree(path)
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
				arch.update(arch.SaveStats, Stats{Files: 1})
			}(node)
		} else {
			arch.update(arch.SaveStats, Stats{Other: 1})
		}
	}

	wg.Wait()

	blob, err := arch.SaveJSON(backend.Tree, t)
	if err != nil {
		return Blob{}, err
	}

	return blob, nil
}

func (arch *Archiver) Snapshot(dir string, t *Tree) (*Snapshot, backend.ID, error) {
	// reset global stats
	arch.updateStats = Stats{}

	sn := NewSnapshot(dir)

	blob, err := arch.saveTree(t)
	if err != nil {
		return nil, nil, err
	}

	sn.Content = blob.ID

	// save snapshot
	sn.BlobList = arch.bl
	blob, err = arch.SaveJSON(backend.Snapshot, sn)
	if err != nil {
		return nil, nil, err
	}

	return sn, blob.Storage, nil
}
