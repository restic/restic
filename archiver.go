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
	c *Cache

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

	// init cache
	arch.c, err = NewCache(s)
	if err != nil {
		return nil, err
	}

	// abort on all errors
	arch.Error = func(string, os.FileInfo, error) error { return err }
	// allow all files
	arch.Filter = func(string, os.FileInfo) bool { return true }

	return arch, nil
}

// Cache returns the current cache for the Archiver.
func (arch *Archiver) Cache() *Cache {
	return arch.c
}

// Preload loads all blobs for all cached snapshots.
func (arch *Archiver) Preload() error {
	done := make(chan struct{})
	defer close(done)

	// list snapshots
	// TODO: track seen tree ids, load trees that aren't in the set
	snapshots := 0
	for name := range arch.s.List(backend.Snapshot, done) {
		id, err := backend.ParseID(name)
		if err != nil {
			debug.Log("Archiver.Preload", "unable to parse name %v as id: %v", name, err)
			continue
		}

		m, err := arch.c.LoadMap(arch.s, id)
		if err != nil {
			debug.Log("Archiver.Preload", "blobs for snapshot %v not cached: %v", id.Str(), err)
			continue
		}

		arch.m.Merge(m)
		debug.Log("Archiver.Preload", "done loading cached blobs for snapshot %v", id.Str())
		snapshots++
	}

	debug.Log("Archiver.Preload", "Loaded %v blobs from %v snapshots", arch.m.Len(), snapshots)
	return nil
}

func (arch *Archiver) Save(t backend.Type, id backend.ID, length uint, rd io.Reader) (Blob, error) {
	debug.Log("Archiver.Save", "Save(%v, %v)\n", t, id.Str())

	// test if this blob is already known
	blob, err := arch.m.FindID(id)
	if err == nil {
		debug.Log("Archiver.Save", "Save(%v, %v): reusing %v\n", t, id.Str(), blob.Storage.Str())
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
		err = arch.s.Remove(t, blob.Storage.String())
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
	file, err := node.OpenForReading()
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
				debug.Log("Archiver.SaveFile", "NodeFromFileInfo returned error for %v: %v", node.path, err)
				return nil, err
			}

			// copy node
			*node = *n
		}
	}

	var blobs Blobs

	// store all chunks
	chnker := GetChunker("archiver.SaveFile")
	chnker.Reset(file, arch.s.ChunkerPolynomial())
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

					if ok, err := arch.s.Test(backend.Data, blob.Storage.String()); !ok || err != nil {
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
	defer func() {
		debug.Log("Archiver.fileWorker", "done")
		wg.Done()
	}()
	for {
		select {
		case e, ok := <-entCh:
			if !ok {
				// channel is closed
				return
			}

			debug.Log("Archiver.fileWorker", "got job %v", e)

			// check for errors
			if e.Error() != nil {
				debug.Log("Archiver.fileWorker", "job %v has errors: %v", e.Path(), e.Error())
				// TODO: integrate error reporting
				fmt.Fprintf(os.Stderr, "error for %v: %v\n", e.Path(), e.Error())
				// ignore this file
				e.Result() <- nil
				p.Report(Stat{Files: 1})
				continue
			}

			node, err := NodeFromFileInfo(e.Fullpath(), e.Info())
			if err != nil {
				// TODO: integrate error reporting
				debug.Log("Archiver.fileWorker", "NodeFromFileInfo returned error for %v: %v", node.path, err)
				e.Result() <- nil
				p.Report(Stat{Files: 1})
				continue
			}

			// try to use old node, if present
			if e.Node != nil {
				debug.Log("Archiver.fileWorker", "   %v use old data", e.Path())

				oldNode := e.Node.(*Node)
				// check if all content is still available in the repository
				contentMissing := false
				for _, blob := range oldNode.blobs {
					if ok, err := arch.s.Test(backend.Data, blob.Storage.String()); !ok || err != nil {
						debug.Log("Archiver.fileWorker", "   %v not using old data, %v (%v) is missing", e.Path(), blob.ID.Str(), blob.Storage.Str())
						contentMissing = true
						break
					}
				}

				if !contentMissing {
					node.Content = oldNode.Content
					node.blobs = oldNode.blobs
					debug.Log("Archiver.fileWorker", "   %v content is complete", e.Path())
				}
			} else {
				debug.Log("Archiver.fileWorker", "   %v no old data", e.Path())
			}

			// otherwise read file normally
			if node.Type == "file" && len(node.Content) == 0 {
				debug.Log("Archiver.fileWorker", "   read and save %v, content: %v", e.Path(), node.Content)
				node.blobs, err = arch.SaveFile(p, node)
				if err != nil {
					// TODO: integrate error reporting
					fmt.Fprintf(os.Stderr, "error for %v: %v\n", node.path, err)
					// ignore this file
					e.Result() <- nil
					p.Report(Stat{Files: 1})
					continue
				}
			} else {
				// report old data size
				p.Report(Stat{Bytes: node.Size})
			}

			debug.Log("Archiver.fileWorker", "   processed %v, %d/%d blobs", e.Path(), len(node.Content), len(node.blobs))
			e.Result() <- node
			p.Report(Stat{Files: 1})
		case <-done:
			// pipeline was cancelled
			return
		}
	}
}

func (arch *Archiver) dirWorker(wg *sync.WaitGroup, p *Progress, done <-chan struct{}, dirCh <-chan pipe.Dir) {
	defer func() {
		debug.Log("Archiver.dirWorker", "done")
		wg.Done()
	}()
	for {
		select {
		case dir, ok := <-dirCh:
			if !ok {
				// channel is closed
				return
			}
			debug.Log("Archiver.dirWorker", "save dir %v\n", dir.Path())

			tree := NewTree()

			// wait for all content
			for _, ch := range dir.Entries {
				res := <-ch

				// if we get a nil pointer here, an error has happened while
				// processing this entry. Ignore it for now.
				if res == nil {
					continue
				}

				// else insert node
				node := res.(*Node)
				tree.Insert(node)

				if node.Type == "dir" {
					debug.Log("Archiver.dirWorker", "got tree node for %s: %v", node.path, node.blobs)
				}

				// also store blob in tree map
				for _, blob := range node.blobs {
					tree.Map.Insert(blob)
				}
			}

			var (
				node *Node
				err  error
			)
			if dir.Path() == "" {
				// if this is the top-level dir, only create a stub node
				node = &Node{}
			} else {
				// else create node from path and fi
				node, err = NodeFromFileInfo(dir.Path(), dir.Info())
				if err != nil {
					node.Error = err.Error()
					dir.Result() <- node
					continue
				}
			}

			blob, err := arch.SaveTreeJSON(tree)
			if err != nil {
				panic(err)
			}
			debug.Log("Archiver.dirWorker", "save tree for %s: %v", dir.Path(), blob)

			node.Subtree = blob.ID
			node.blobs = Blobs{blob}

			dir.Result() <- node
			if dir.Path() != "" {
				p.Report(Stat{Dirs: 1})
			}
		case <-done:
			// pipeline was cancelled
			return
		}
	}
}

type ArchivePipe struct {
	Old <-chan WalkTreeJob
	New <-chan pipe.Job
}

func copyJobs(done <-chan struct{}, in <-chan pipe.Job, out chan<- pipe.Job) {
	i := in
	o := out

	o = nil

	var (
		j  pipe.Job
		ok bool
	)
	for {
		select {
		case <-done:
			return
		case j, ok = <-i:
			if !ok {
				// in ch closed, we're done
				debug.Log("copyJobs", "in channel closed, we're done")
				return
			}
			i = nil
			o = out
		case o <- j:
			o = nil
			i = in
		}
	}
}

type archiveJob struct {
	hasOld bool
	old    WalkTreeJob
	new    pipe.Job
}

func (a *ArchivePipe) compare(done <-chan struct{}, out chan<- pipe.Job) {
	defer func() {
		close(out)
		debug.Log("ArchivePipe.compare", "done")
	}()

	debug.Log("ArchivePipe.compare", "start")
	var (
		loadOld, loadNew bool = true, true
		ok               bool
		oldJob           WalkTreeJob
		newJob           pipe.Job
	)

	for {
		if loadOld {
			oldJob, ok = <-a.Old
			// if the old channel is closed, just pass through the new jobs
			if !ok {
				debug.Log("ArchivePipe.compare", "old channel is closed, copy from new channel")

				// handle remaining newJob
				if !loadNew {
					out <- archiveJob{new: newJob}.Copy()
				}

				copyJobs(done, a.New, out)
				return
			}

			loadOld = false
		}

		if loadNew {
			newJob, ok = <-a.New
			// if the new channel is closed, there are no more files in the current snapshot, return
			if !ok {
				debug.Log("ArchivePipe.compare", "new channel is closed, we're done")
				return
			}

			loadNew = false
		}

		debug.Log("ArchivePipe.compare", "old job: %v", oldJob.Path)
		debug.Log("ArchivePipe.compare", "new job: %v", newJob.Path())

		// at this point we have received an old job as well as a new job, compare paths
		file1 := oldJob.Path
		file2 := newJob.Path()

		dir1 := filepath.Dir(file1)
		dir2 := filepath.Dir(file2)

		if file1 == file2 {
			debug.Log("ArchivePipe.compare", "    same filename %q", file1)

			// send job
			out <- archiveJob{hasOld: true, old: oldJob, new: newJob}.Copy()
			loadOld = true
			loadNew = true
			continue
		} else if dir1 < dir2 {
			debug.Log("ArchivePipe.compare", "    %q < %q, file %q added", dir1, dir2, file2)
			// file is new, send new job and load new
			loadNew = true
			out <- archiveJob{new: newJob}.Copy()
			continue
		} else if dir1 == dir2 {
			if file1 < file2 {
				debug.Log("ArchivePipe.compare", "    %q < %q, file %q removed", file1, file2, file1)
				// file has been removed, load new old
				loadOld = true
				continue
			} else {
				debug.Log("ArchivePipe.compare", "    %q > %q, file %q added", file1, file2, file2)
				// file is new, send new job and load new
				loadNew = true
				out <- archiveJob{new: newJob}.Copy()
				continue
			}
		}

		debug.Log("ArchivePipe.compare", "    %q > %q, file %q removed", file1, file2, file1)
		// file has been removed, throw away old job and load new
		loadOld = true
	}
}

func (j archiveJob) Copy() pipe.Job {
	if !j.hasOld {
		return j.new
	}

	// handle files
	if isFile(j.new.Info()) {
		debug.Log("archiveJob.Copy", "   job %v is file", j.new.Path())

		// if type has changed, return new job directly
		if j.old.Node == nil {
			return j.new
		}

		// if file is newer, return the new job
		if j.old.Node.isNewer(j.new.Fullpath(), j.new.Info()) {
			debug.Log("archiveJob.Copy", "   job %v is newer", j.new.Path())
			return j.new
		}

		debug.Log("archiveJob.Copy", "   job %v add old data", j.new.Path())
		// otherwise annotate job with old data
		e := j.new.(pipe.Entry)
		e.Node = j.old.Node
		return e
	}

	// dirs and other types are just returned
	return j.new
}

func (arch *Archiver) Snapshot(p *Progress, paths []string, pid backend.ID) (*Snapshot, backend.ID, error) {
	debug.Log("Archiver.Snapshot", "start for %v", paths)

	debug.Break("Archiver.Snapshot")
	sort.Strings(paths)

	// signal the whole pipeline to stop
	done := make(chan struct{})
	var err error

	p.Start()
	defer p.Done()

	// create new snapshot
	sn, err := NewSnapshot(paths)
	if err != nil {
		return nil, nil, err
	}

	jobs := ArchivePipe{}

	// use parent snapshot (if some was given)
	if pid != nil {
		sn.Parent = pid

		// load parent snapshot
		parent, err := LoadSnapshot(arch.s, pid)
		if err != nil {
			return nil, nil, err
		}

		// start walker on old tree
		ch := make(chan WalkTreeJob)
		go WalkTree(arch.s, parent.Tree, done, ch)
		jobs.Old = ch
	} else {
		// use closed channel
		ch := make(chan WalkTreeJob)
		close(ch)
		jobs.Old = ch
	}

	// start walker
	pipeCh := make(chan pipe.Job)
	resCh := make(chan pipe.Result, 1)
	go func() {
		err := pipe.Walk(paths, done, pipeCh, resCh)
		if err != nil {
			debug.Log("Archiver.Snapshot", "pipe.Walk returned error %v", err)
			return
		}
		debug.Log("Archiver.Snapshot", "pipe.Walk done")
	}()
	jobs.New = pipeCh

	ch := make(chan pipe.Job)
	go jobs.compare(done, ch)

	var wg sync.WaitGroup
	entCh := make(chan pipe.Entry)
	dirCh := make(chan pipe.Dir)

	// split
	wg.Add(1)
	go func() {
		pipe.Split(ch, dirCh, entCh)
		debug.Log("Archiver.Snapshot", "split done")
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

	// wait for all workers to terminate
	debug.Log("Archiver.Snapshot", "wait for workers")
	wg.Wait()

	debug.Log("Archiver.Snapshot", "workers terminated")

	// receive the top-level tree
	root := (<-resCh).(*Node)
	blob := root.blobs[0]
	debug.Log("Archiver.Snapshot", "root node received: %v", blob)
	sn.Tree = blob

	// save snapshot
	blob, err = arch.s.SaveJSON(backend.Snapshot, sn)
	if err != nil {
		return nil, nil, err
	}

	// store ID in snapshot struct
	sn.id = blob.Storage

	debug.Log("Archiver.Snapshot", "saved snapshot %v", blob.Storage.Str())

	// cache blobs
	err = arch.c.StoreMap(sn.id, arch.m)
	if err != nil {
		debug.Log("Archiver.Snapshot", "unable to cache blobs for snapshot %v: %v", blob.Storage.Str(), err)
		fmt.Fprintf(os.Stderr, "unable to cache blobs for snapshot %v: %v\n", blob.Storage.Str(), err)
		return sn, blob.Storage, nil
	}

	return sn, blob.Storage, nil
}

func isFile(fi os.FileInfo) bool {
	if fi == nil {
		return false
	}

	return fi.Mode()&(os.ModeType|os.ModeCharDevice) == 0
}

func Scan(dirs []string, p *Progress) (Stat, error) {
	p.Start()
	defer p.Done()

	var stat Stat

	for _, dir := range dirs {
		debug.Log("Scan", "Start for %v", dir)
		err := filepath.Walk(dir, func(str string, fi os.FileInfo, err error) error {
			debug.Log("Scan.Walk", "%v, fi: %v, err: %v", str, fi, err)
			// TODO: integrate error reporting
			if err != nil {
				fmt.Fprintf(os.Stderr, "error for %v: %v\n", str, err)
				return nil
			}
			if fi == nil {
				fmt.Fprintf(os.Stderr, "error for %v: FileInfo is nil\n", str)
				return nil
			}
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

		debug.Log("Scan", "Done for %v, err: %v", dir, err)
		if err != nil {
			return Stat{}, err
		}
	}

	return stat, nil
}
