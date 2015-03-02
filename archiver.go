package restic

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/juju/arrar"
	"github.com/restic/restic/backend"
	"github.com/restic/restic/chunker"
	"github.com/restic/restic/debug"
	"github.com/restic/restic/pipe"
)

const (
	maxConcurrentBlobs    = 32
	maxConcurrency        = 10
	maxConcurrencyPreload = 20

	// chunkerBufSize is used in pool.go
	chunkerBufSize = 512 * chunker.KiB
)

type Archiver struct {
	s Server
	m *Map

	blobToken chan struct{}

	Error  func(dir string, fi os.FileInfo, err error) error
	Filter func(item string, fi os.FileInfo) bool
}

func NewArchiver(s Server) (*Archiver, error) {
	var err error
	arch := &Archiver{
		s:         s,
		blobToken: make(chan struct{}, maxConcurrentBlobs),
	}

	// fill blob token
	for i := 0; i < maxConcurrentBlobs; i++ {
		arch.blobToken <- struct{}{}
	}

	// create new map to store all blobs in
	arch.m = NewMap()

	// abort on all errors
	arch.Error = func(string, os.FileInfo, error) error { return err }
	// allow all files
	arch.Filter = func(string, os.FileInfo) bool { return true }

	return arch, nil
}

// Preload loads all tree objects from repository and adds all blobs that are
// still available to the map for deduplication.
func (arch *Archiver) Preload(p *Progress) error {
	cache, err := NewCache()
	if err != nil {
		return err
	}

	p.Start()
	defer p.Done()

	debug.Log("Archiver.Preload", "Start loading known blobs")

	// load all trees, in parallel
	worker := func(wg *sync.WaitGroup, c <-chan backend.ID) {
		for id := range c {
			var tree *Tree

			// load from cache
			var t Tree
			rd, err := cache.Load(backend.Tree, id)
			if err == nil {
				debug.Log("Archiver.Preload", "tree %v cached", id.Str())
				tree = &t
				dec := json.NewDecoder(rd)
				err = dec.Decode(&t)

				if err != nil {
					continue
				}
			} else {
				debug.Log("Archiver.Preload", "tree %v not cached: %v", id.Str(), err)

				tree, err = LoadTree(arch.s, id)
				// ignore error and advance to next tree
				if err != nil {
					continue
				}
			}

			debug.Log("Archiver.Preload", "load tree %v with %d blobs", id, tree.Map.Len())
			arch.m.Merge(tree.Map)
			p.Report(Stat{Trees: 1, Blobs: uint64(tree.Map.Len())})
		}
		wg.Done()
	}

	idCh := make(chan backend.ID)

	// start workers
	var wg sync.WaitGroup
	for i := 0; i < maxConcurrencyPreload; i++ {
		wg.Add(1)
		go worker(&wg, idCh)
	}

	// list ids
	trees := 0
	err = arch.s.EachID(backend.Tree, func(id backend.ID) {
		trees++

		if trees%1000 == 0 {
			debug.Log("Archiver.Preload", "Loaded %v trees", trees)
		}
		idCh <- id
	})

	close(idCh)

	// wait for workers
	wg.Wait()

	debug.Log("Archiver.Preload", "Loaded %v blobs from %v trees", arch.m.Len(), trees)

	return err
}

func (arch *Archiver) Save(t backend.Type, id backend.ID, length uint, rd io.Reader) (Blob, error) {
	debug.Log("Archiver.Save", "Save(%v, %v)\n", t, id.Str())

	// test if this blob is already known
	blob, err := arch.m.FindID(id)
	if err == nil {
		debug.Log("Archiver.Save", "Save(%v, %v): reusing %v\n", t, id.Str(), blob.Storage.Str())
		id.Free()
		return blob, nil
	}

	// else encrypt and save data
	blob, err = arch.s.SaveFrom(t, id, length, rd)

	// store blob in storage map
	smapblob := arch.m.Insert(blob)

	// if the map has a different storage id for this plaintext blob, use that
	// one and remove the other. This happens if the same plaintext blob was
	// stored concurrently and finished earlier than this blob.
	if blob.Storage.Compare(smapblob.Storage) != 0 {
		debug.Log("Archiver.Save", "using other block, removing %v\n", blob.Storage.Str())

		// remove the blob again
		// TODO: implement a list of blobs in transport, so this doesn't happen so often
		err = arch.s.Remove(t, blob.Storage)
		if err != nil {
			return Blob{}, err
		}
	}

	debug.Log("Archiver.Save", "Save(%v, %v): new blob %v\n", t, id.Str(), blob)

	return smapblob, nil
}

func (arch *Archiver) SaveTreeJSON(item interface{}) (Blob, error) {
	// convert to json
	data, err := json.Marshal(item)
	// append newline
	data = append(data, '\n')
	if err != nil {
		return Blob{}, err
	}

	// check if tree has been saved before
	id := backend.Hash(data)
	blob, err := arch.m.FindID(id)

	// return the blob if we found it
	if err == nil {
		return blob, nil
	}

	// otherwise save the data
	blob, err = arch.s.SaveJSON(backend.Tree, item)
	if err != nil {
		return Blob{}, err
	}

	// store blob in storage map
	arch.m.Insert(blob)

	return blob, nil
}

// SaveFile stores the content of the file on the backend as a Blob by calling
// Save for each chunk.
func (arch *Archiver) SaveFile(p *Progress, node *Node) (Blobs, error) {
	file, err := os.Open(node.path)
	defer file.Close()
	if err != nil {
		return nil, err
	}

	// check file again
	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if fi.ModTime() != node.ModTime {
		e2 := arch.Error(node.path, fi, errors.New("file was updated, using new version"))

		if e2 == nil {
			// create new node
			n, err := NodeFromFileInfo(node.path, fi)
			if err != nil {
				return nil, err
			}

			// copy node
			*node = *n
		}
	}

	var blobs Blobs

	// store all chunks
	chnker := GetChunker("archiver.SaveFile")
	chnker.Reset(file)
	chans := [](<-chan Blob){}
	defer FreeChunker("archiver.SaveFile", chnker)

	chunks := 0

	for {
		chunk, err := chnker.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, arrar.Annotate(err, "SaveFile() chunker.Next()")
		}

		chunks++

		// acquire token, start goroutine to save chunk
		token := <-arch.blobToken
		resCh := make(chan Blob, 1)

		go func(ch chan<- Blob) {
			blob, err := arch.Save(backend.Data, chunk.Digest, chunk.Length, chunk.Reader(file))
			// TODO handle error
			if err != nil {
				panic(err)
			}

			p.Report(Stat{Bytes: blob.Size})
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
		return nil, fmt.Errorf("chunker returned %v chunks, but only %v blobs saved", chunks, len(blobs))
	}

	var bytes uint64

	node.Content = make([]backend.ID, len(blobs))
	debug.Log("Archiver.Save", "checking size for file %s", node.path)
	for i, blob := range blobs {
		node.Content[i] = blob.ID
		bytes += blob.Size

		debug.Log("Archiver.Save", "  adding blob %s", blob)
	}

	if bytes != node.Size {
		return nil, fmt.Errorf("errors saving node %q: saved %d bytes, wanted %d bytes", node.path, bytes, node.Size)
	}

	debug.Log("Archiver.SaveFile", "SaveFile(%q): %v\n", node.path, blobs)

	return blobs, nil
}

func (arch *Archiver) saveTree(p *Progress, t *Tree) (Blob, error) {
	debug.Log("Archiver.saveTree", "saveTree(%v)\n", t)
	var wg sync.WaitGroup

	// add all blobs to global map
	arch.m.Merge(t.Map)

	// TODO: do all this in parallel
	for _, node := range t.Nodes {
		if node.tree != nil {
			b, err := arch.saveTree(p, node.tree)
			if err != nil {
				return Blob{}, err
			}
			node.Subtree = b.ID
			t.Map.Insert(b)
			p.Report(Stat{Dirs: 1})
		} else if node.Type == "file" {
			if len(node.Content) > 0 {
				removeContent := false

				// check content
				for _, id := range node.Content {
					blob, err := t.Map.FindID(id)
					if err != nil {
						debug.Log("Archiver.saveTree", "unable to find storage id for data blob %v", id.Str())
						arch.Error(node.path, nil, fmt.Errorf("unable to find storage id for data blob %v", id.Str()))
						removeContent = true
						t.Map.DeleteID(id)
						arch.m.DeleteID(id)
						continue
					}

					if ok, err := arch.s.Test(backend.Data, blob.Storage); !ok || err != nil {
						debug.Log("Archiver.saveTree", "blob %v not in repository (error is %v)", blob, err)
						arch.Error(node.path, nil, fmt.Errorf("blob %v not in repository (error is %v)", blob.Storage.Str(), err))
						removeContent = true
						t.Map.DeleteID(id)
						arch.m.DeleteID(id)
					}
				}

				if removeContent {
					debug.Log("Archiver.saveTree", "removing content for %s", node.path)
					node.Content = node.Content[:0]
				}
			}

			if len(node.Content) == 0 {
				// start goroutine
				wg.Add(1)
				go func(n *Node) {
					defer wg.Done()

					var blobs Blobs
					blobs, n.err = arch.SaveFile(p, n)
					for _, b := range blobs {
						t.Map.Insert(b)
					}

					p.Report(Stat{Files: 1})
				}(node)
			}
		}
	}

	wg.Wait()

	usedIDs := backend.NewIDSet()

	// check for invalid file nodes
	for _, node := range t.Nodes {
		if node.Type == "file" && node.Content == nil && node.err == nil {
			return Blob{}, fmt.Errorf("node %v has empty content", node.Name)
		}

		// remember used hashes
		if node.Type == "file" && node.Content != nil {
			for _, id := range node.Content {
				usedIDs.Insert(id)
			}
		}

		if node.Type == "dir" && node.Subtree != nil {
			usedIDs.Insert(node.Subtree)
		}

		if node.err != nil {
			err := arch.Error(node.path, nil, node.err)
			if err != nil {
				return Blob{}, err
			}

			// save error message in node
			node.Error = node.err.Error()
		}
	}

	before := len(t.Map.IDs())
	t.Map.Prune(usedIDs)
	after := len(t.Map.IDs())

	if before != after {
		debug.Log("Archiver.saveTree", "pruned %d ids from map for tree %v\n", before-after, t)
	}

	blob, err := arch.SaveTreeJSON(t)
	if err != nil {
		return Blob{}, err
	}

	return blob, nil
}

func (arch *Archiver) fileWorker(wg *sync.WaitGroup, p *Progress, done <-chan struct{}, entCh <-chan pipe.Entry) {
	defer wg.Done()
	for {
		select {
		case e, ok := <-entCh:
			if !ok {
				// channel is closed
				return
			}

			node, err := NodeFromFileInfo(e.Path, e.Info)
			if err != nil {
				panic(err)
			}

			if node.Type == "file" {
				node.blobs, err = arch.SaveFile(p, node)
				if err != nil {
					panic(err)
				}
			}

			e.Result <- node
			p.Report(Stat{Files: 1})
		case <-done:
			// pipeline was cancelled
			return
		}
	}
}

func (arch *Archiver) dirWorker(wg *sync.WaitGroup, p *Progress, done <-chan struct{}, dirCh <-chan pipe.Dir) {
	defer wg.Done()
	for {
		select {
		case dir, ok := <-dirCh:
			if !ok {
				// channel is closed
				return
			}
			debug.Log("Archiver.DirWorker", "save dir %v\n", dir.Path)

			tree := NewTree()

			// wait for all content
			for _, ch := range dir.Entries {
				node := (<-ch).(*Node)
				tree.Insert(node)

				if node.Type == "dir" {
					debug.Log("Archiver.DirWorker", "got tree node for %s: %v", node.path, node.blobs)
				}

				for _, blob := range node.blobs {
					tree.Map.Insert(blob)
					arch.m.Insert(blob)
				}
			}

			node, err := NodeFromFileInfo(dir.Path, dir.Info)
			if err != nil {
				node.Error = err.Error()
				dir.Result <- node
				continue
			}

			blob, err := arch.SaveTreeJSON(tree)
			if err != nil {
				panic(err)
			}
			debug.Log("Archiver.DirWorker", "save tree for %s: %v", dir.Path, blob)

			node.Subtree = blob.ID
			node.blobs = Blobs{blob}

			dir.Result <- node
			p.Report(Stat{Dirs: 1})
		case <-done:
			// pipeline was cancelled
			return
		}
	}
}

func compareWithOldTree(newCh <-chan interface{}, oldCh <-chan WalkTreeJob, outCh chan<- interface{}) {
	debug.Log("Archiver.compareWithOldTree", "start")
	defer func() {
		debug.Log("Archiver.compareWithOldTree", "done")
	}()
	for {
		debug.Log("Archiver.compareWithOldTree", "waiting for new job")
		newJob, ok := <-newCh
		if !ok {
			// channel is closed
			return
		}

		debug.Log("Archiver.compareWithOldTree", "received new job %v", newJob)
		oldJob, ok := <-oldCh
		if !ok {
			// channel is closed
			return
		}

		debug.Log("Archiver.compareWithOldTree", "received old job %v", oldJob)

		outCh <- newJob
	}
}

func (arch *Archiver) Snapshot(p *Progress, paths []string, parentSnapshot backend.ID) (*Snapshot, backend.ID, error) {
	debug.Log("Archiver.Snapshot", "start for %v", paths)

	debug.Break("Archiver.Snapshot")
	sort.Strings(paths)

	p.Start()
	defer p.Done()

	sn, err := NewSnapshot(paths)
	if err != nil {
		return nil, nil, err
	}

	// load parent snapshot
	// var oldRoot backend.ID
	// if parentSnapshot != nil {
	// 	sn.Parent = parentSnapshot
	// 	parentSn, err := LoadSnapshot(arch.s, parentSnapshot)
	// 	if err != nil {
	// 		return nil, nil, err
	// 	}
	// 	oldRoot = parentSn.Tree.Storage
	// }

	// signal the whole pipeline to stop
	done := make(chan struct{})

	// if we have an old root, start walker and comparer
	// oldTreeCh := make(chan WalkTreeJob)
	// if oldRoot != nil {
	// 	// start walking the old tree
	// 	debug.Log("Archiver.Snapshot", "start comparer for old root %v", oldRoot.Str())
	// 	go WalkTree(arch.s, oldRoot, done, oldTreeCh)
	// }

	var wg sync.WaitGroup
	entCh := make(chan pipe.Entry)
	dirCh := make(chan pipe.Dir)
	jobsCh := make(chan interface{})

	// split
	wg.Add(1)
	go func() {
		pipe.Split(jobsCh, dirCh, entCh)
		close(dirCh)
		close(entCh)
		wg.Done()
	}()

	// run workers
	for i := 0; i < maxConcurrency; i++ {
		wg.Add(2)
		go arch.fileWorker(&wg, p, done, entCh)
		go arch.dirWorker(&wg, p, done, dirCh)
	}

	// start walker
	resCh, err := pipe.Walk(paths, done, jobsCh)
	if err != nil {
		close(done)

		debug.Log("Archiver.Snapshot", "pipe.Walke returned error %v", err)
		return nil, nil, err
	}

	// wait for all workers to terminate
	debug.Log("Archiver.Snapshot", "wait for workers")
	wg.Wait()

	debug.Log("Archiver.Snapshot", "workers terminated")

	// add the top-level tree
	tree := NewTree()
	root := (<-resCh).(pipe.Dir)
	for i := 0; i < len(paths); i++ {
		node := (<-root.Entries[i]).(*Node)

		debug.Log("Archiver.Snapshot", "got toplevel node %v", node)

		tree.Insert(node)
		for _, blob := range node.blobs {
			blob = arch.m.Insert(blob)
			tree.Map.Insert(blob)
		}
	}

	tb, err := arch.SaveTreeJSON(tree)
	if err != nil {
		return nil, nil, err
	}

	sn.Tree = tb

	// save snapshot
	blob, err := arch.s.SaveJSON(backend.Snapshot, sn)
	if err != nil {
		return nil, nil, err
	}

	return sn, blob.Storage, nil
}

func isFile(fi os.FileInfo) bool {
	return fi.Mode()&(os.ModeType|os.ModeCharDevice) == 0
}

func Scan(dirs []string, p *Progress) (Stat, error) {
	p.Start()
	defer p.Done()

	var stat Stat

	for _, dir := range dirs {
		err := filepath.Walk(dir, func(str string, fi os.FileInfo, err error) error {
			s := Stat{}
			if isFile(fi) {
				s.Files++
				s.Bytes += uint64(fi.Size())
			} else if fi.IsDir() {
				s.Dirs++
			}

			p.Report(s)
			stat.Add(s)

			// TODO: handle error?
			return nil
		})

		if err != nil {
			return Stat{}, err
		}
	}

	return stat, nil
}
