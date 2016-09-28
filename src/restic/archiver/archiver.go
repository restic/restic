package archiver

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"restic"
	"sort"
	"sync"
	"time"

	"restic/errors"
	"restic/walk"

	"restic/debug"
	"restic/fs"
	"restic/pipe"

	"github.com/restic/chunker"
)

const (
	maxConcurrentBlobs = 32
	maxConcurrency     = 10
)

var archiverAbortOnAllErrors = func(str string, fi os.FileInfo, err error) error { return err }
var archiverAllowAllFiles = func(string, os.FileInfo) bool { return true }

// Archiver is used to backup a set of directories.
type Archiver struct {
	repo       restic.Repository
	knownBlobs struct {
		restic.IDSet
		sync.Mutex
	}

	blobToken chan struct{}

	Error        func(dir string, fi os.FileInfo, err error) error
	SelectFilter pipe.SelectFunc
	Excludes     []string
}

// New returns a new archiver.
func New(repo restic.Repository) *Archiver {
	arch := &Archiver{
		repo:      repo,
		blobToken: make(chan struct{}, maxConcurrentBlobs),
		knownBlobs: struct {
			restic.IDSet
			sync.Mutex
		}{
			IDSet: restic.NewIDSet(),
		},
	}

	for i := 0; i < maxConcurrentBlobs; i++ {
		arch.blobToken <- struct{}{}
	}

	arch.Error = archiverAbortOnAllErrors
	arch.SelectFilter = archiverAllowAllFiles

	return arch
}

// isKnownBlob returns true iff the blob is not yet in the list of known blobs.
// When the blob is not known, false is returned and the blob is added to the
// list. This means that the caller false is returned to is responsible to save
// the blob to the backend.
func (arch *Archiver) isKnownBlob(id restic.ID, t restic.BlobType) bool {
	arch.knownBlobs.Lock()
	defer arch.knownBlobs.Unlock()

	if arch.knownBlobs.Has(id) {
		return true
	}

	arch.knownBlobs.Insert(id)

	_, err := arch.repo.Index().Lookup(id, t)
	if err == nil {
		return true
	}

	return false
}

// Save stores a blob read from rd in the repository.
func (arch *Archiver) Save(t restic.BlobType, data []byte, id restic.ID) error {
	debug.Log("Save(%v, %v)\n", t, id.Str())

	if arch.isKnownBlob(id, restic.DataBlob) {
		debug.Log("blob %v is known\n", id.Str())
		return nil
	}

	_, err := arch.repo.SaveBlob(t, data, id)
	if err != nil {
		debug.Log("Save(%v, %v): error %v\n", t, id.Str(), err)
		return err
	}

	debug.Log("Save(%v, %v): new blob\n", t, id.Str())
	return nil
}

// SaveTreeJSON stores a tree in the repository.
func (arch *Archiver) SaveTreeJSON(tree *restic.Tree) (restic.ID, error) {
	data, err := json.Marshal(tree)
	if err != nil {
		return restic.ID{}, errors.Wrap(err, "Marshal")
	}
	data = append(data, '\n')

	// check if tree has been saved before
	id := restic.Hash(data)
	if arch.isKnownBlob(id, restic.TreeBlob) {
		return id, nil
	}

	return arch.repo.SaveBlob(restic.TreeBlob, data, id)
}

func (arch *Archiver) reloadFileIfChanged(node *restic.Node, file fs.File) (*restic.Node, error) {
	fi, err := file.Stat()
	if err != nil {
		return nil, errors.Wrap(err, "restic.Stat")
	}

	if fi.ModTime() == node.ModTime {
		return node, nil
	}

	err = arch.Error(node.Path, fi, errors.New("file has changed"))
	if err != nil {
		return nil, err
	}

	node, err = restic.NodeFromFileInfo(node.Path, fi)
	if err != nil {
		debug.Log("restic.NodeFromFileInfo returned error for %v: %v", node.Path, err)
		return nil, err
	}

	return node, nil
}

type saveResult struct {
	id    restic.ID
	bytes uint64
}

func (arch *Archiver) saveChunk(chunk chunker.Chunk, p *restic.Progress, token struct{}, file fs.File, resultChannel chan<- saveResult) {
	defer freeBuf(chunk.Data)

	id := restic.Hash(chunk.Data)
	err := arch.Save(restic.DataBlob, chunk.Data, id)
	// TODO handle error
	if err != nil {
		panic(err)
	}

	p.Report(restic.Stat{Bytes: uint64(chunk.Length)})
	arch.blobToken <- token
	resultChannel <- saveResult{id: id, bytes: uint64(chunk.Length)}
}

func waitForResults(resultChannels [](<-chan saveResult)) ([]saveResult, error) {
	results := []saveResult{}

	for _, ch := range resultChannels {
		results = append(results, <-ch)
	}

	if len(results) != len(resultChannels) {
		return nil, errors.Errorf("chunker returned %v chunks, but only %v blobs saved", len(resultChannels), len(results))
	}

	return results, nil
}

func updateNodeContent(node *restic.Node, results []saveResult) error {
	debug.Log("checking size for file %s", node.Path)

	var bytes uint64
	node.Content = make([]restic.ID, len(results))

	for i, b := range results {
		node.Content[i] = b.id
		bytes += b.bytes

		debug.Log("  adding blob %s, %d bytes", b.id.Str(), b.bytes)
	}

	if bytes != node.Size {
		return errors.Errorf("errors saving node %q: saved %d bytes, wanted %d bytes", node.Path, bytes, node.Size)
	}

	debug.Log("SaveFile(%q): %v blobs\n", node.Path, len(results))

	return nil
}

// SaveFile stores the content of the file on the backend as a Blob by calling
// Save for each chunk.
func (arch *Archiver) SaveFile(p *restic.Progress, node *restic.Node) error {
	file, err := fs.Open(node.Path)
	defer file.Close()
	if err != nil {
		return errors.Wrap(err, "Open")
	}

	node, err = arch.reloadFileIfChanged(node, file)
	if err != nil {
		return err
	}

	chnker := chunker.New(file, arch.repo.Config().ChunkerPolynomial)
	resultChannels := [](<-chan saveResult){}

	for {
		chunk, err := chnker.Next(getBuf())
		if errors.Cause(err) == io.EOF {
			break
		}

		if err != nil {
			return errors.Wrap(err, "chunker.Next")
		}

		resCh := make(chan saveResult, 1)
		go arch.saveChunk(chunk, p, <-arch.blobToken, file, resCh)
		resultChannels = append(resultChannels, resCh)
	}

	results, err := waitForResults(resultChannels)
	if err != nil {
		return err
	}

	err = updateNodeContent(node, results)
	return err
}

func (arch *Archiver) fileWorker(wg *sync.WaitGroup, p *restic.Progress, done <-chan struct{}, entCh <-chan pipe.Entry) {
	defer func() {
		debug.Log("done")
		wg.Done()
	}()
	for {
		select {
		case e, ok := <-entCh:
			if !ok {
				// channel is closed
				return
			}

			debug.Log("got job %v", e)

			// check for errors
			if e.Error() != nil {
				debug.Log("job %v has errors: %v", e.Path(), e.Error())
				// TODO: integrate error reporting
				fmt.Fprintf(os.Stderr, "error for %v: %v\n", e.Path(), e.Error())
				// ignore this file
				e.Result() <- nil
				p.Report(restic.Stat{Errors: 1})
				continue
			}

			node, err := restic.NodeFromFileInfo(e.Fullpath(), e.Info())
			if err != nil {
				// TODO: integrate error reporting
				debug.Log("restic.NodeFromFileInfo returned error for %v: %v", node.Path, err)
				e.Result() <- nil
				p.Report(restic.Stat{Errors: 1})
				continue
			}

			// try to use old node, if present
			if e.Node != nil {
				debug.Log("   %v use old data", e.Path())

				oldNode := e.Node.(*restic.Node)
				// check if all content is still available in the repository
				contentMissing := false
				for _, blob := range oldNode.Content {
					if !arch.repo.Index().Has(blob, restic.DataBlob) {
						debug.Log("   %v not using old data, %v is missing", e.Path(), blob.Str())
						contentMissing = true
						break
					}
				}

				if !contentMissing {
					node.Content = oldNode.Content
					debug.Log("   %v content is complete", e.Path())
				}
			} else {
				debug.Log("   %v no old data", e.Path())
			}

			// otherwise read file normally
			if node.Type == "file" && len(node.Content) == 0 {
				debug.Log("   read and save %v, content: %v", e.Path(), node.Content)
				err = arch.SaveFile(p, node)
				if err != nil {
					// TODO: integrate error reporting
					fmt.Fprintf(os.Stderr, "error for %v: %v\n", node.Path, err)
					// ignore this file
					e.Result() <- nil
					p.Report(restic.Stat{Errors: 1})
					continue
				}
			} else {
				// report old data size
				p.Report(restic.Stat{Bytes: node.Size})
			}

			debug.Log("   processed %v, %d blobs", e.Path(), len(node.Content))
			e.Result() <- node
			p.Report(restic.Stat{Files: 1})
		case <-done:
			// pipeline was cancelled
			return
		}
	}
}

func (arch *Archiver) dirWorker(wg *sync.WaitGroup, p *restic.Progress, done <-chan struct{}, dirCh <-chan pipe.Dir) {
	debug.Log("start")
	defer func() {
		debug.Log("done")
		wg.Done()
	}()
	for {
		select {
		case dir, ok := <-dirCh:
			if !ok {
				// channel is closed
				return
			}
			debug.Log("save dir %v (%d entries), error %v\n", dir.Path(), len(dir.Entries), dir.Error())

			// ignore dir nodes with errors
			if dir.Error() != nil {
				fmt.Fprintf(os.Stderr, "error walking dir %v: %v\n", dir.Path(), dir.Error())
				dir.Result() <- nil
				p.Report(restic.Stat{Errors: 1})
				continue
			}

			tree := restic.NewTree()

			// wait for all content
			for _, ch := range dir.Entries {
				debug.Log("receiving result from %v", ch)
				res := <-ch

				// if we get a nil pointer here, an error has happened while
				// processing this entry. Ignore it for now.
				if res == nil {
					debug.Log("got nil result?")
					continue
				}

				// else insert node
				node := res.(*restic.Node)
				tree.Insert(node)

				if node.Type == "dir" {
					debug.Log("got tree node for %s: %v", node.Path, node.Subtree)

					if node.Subtree.IsNull() {
						panic("invalid null subtree restic.ID")
					}
				}
			}

			node := &restic.Node{}

			if dir.Path() != "" && dir.Info() != nil {
				n, err := restic.NodeFromFileInfo(dir.Path(), dir.Info())
				if err != nil {
					n.Error = err.Error()
					dir.Result() <- n
					continue
				}
				node = n
			}

			if err := dir.Error(); err != nil {
				node.Error = err.Error()
			}

			id, err := arch.SaveTreeJSON(tree)
			if err != nil {
				panic(err)
			}
			debug.Log("save tree for %s: %v", dir.Path(), id.Str())
			if id.IsNull() {
				panic("invalid null subtree restic.ID return from SaveTreeJSON()")
			}

			node.Subtree = &id

			debug.Log("sending result to %v", dir.Result())

			dir.Result() <- node
			if dir.Path() != "" {
				p.Report(restic.Stat{Dirs: 1})
			}
		case <-done:
			// pipeline was cancelled
			return
		}
	}
}

type archivePipe struct {
	Old <-chan walk.TreeJob
	New <-chan pipe.Job
}

func copyJobs(done <-chan struct{}, in <-chan pipe.Job, out chan<- pipe.Job) {
	var (
		// disable sending on the outCh until we received a job
		outCh chan<- pipe.Job
		// enable receiving from in
		inCh = in
		job  pipe.Job
		ok   bool
	)

	for {
		select {
		case <-done:
			return
		case job, ok = <-inCh:
			if !ok {
				// input channel closed, we're done
				debug.Log("input channel closed, we're done")
				return
			}
			inCh = nil
			outCh = out
		case outCh <- job:
			outCh = nil
			inCh = in
		}
	}
}

type archiveJob struct {
	hasOld bool
	old    walk.TreeJob
	new    pipe.Job
}

func (a *archivePipe) compare(done <-chan struct{}, out chan<- pipe.Job) {
	defer func() {
		close(out)
		debug.Log("done")
	}()

	debug.Log("start")
	var (
		loadOld, loadNew bool = true, true
		ok               bool
		oldJob           walk.TreeJob
		newJob           pipe.Job
	)

	for {
		if loadOld {
			oldJob, ok = <-a.Old
			// if the old channel is closed, just pass through the new jobs
			if !ok {
				debug.Log("old channel is closed, copy from new channel")

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
				debug.Log("new channel is closed, we're done")
				return
			}

			loadNew = false
		}

		debug.Log("old job: %v", oldJob.Path)
		debug.Log("new job: %v", newJob.Path())

		// at this point we have received an old job as well as a new job, compare paths
		file1 := oldJob.Path
		file2 := newJob.Path()

		dir1 := filepath.Dir(file1)
		dir2 := filepath.Dir(file2)

		if file1 == file2 {
			debug.Log("    same filename %q", file1)

			// send job
			out <- archiveJob{hasOld: true, old: oldJob, new: newJob}.Copy()
			loadOld = true
			loadNew = true
			continue
		} else if dir1 < dir2 {
			debug.Log("    %q < %q, file %q added", dir1, dir2, file2)
			// file is new, send new job and load new
			loadNew = true
			out <- archiveJob{new: newJob}.Copy()
			continue
		} else if dir1 == dir2 {
			if file1 < file2 {
				debug.Log("    %q < %q, file %q removed", file1, file2, file1)
				// file has been removed, load new old
				loadOld = true
				continue
			} else {
				debug.Log("    %q > %q, file %q added", file1, file2, file2)
				// file is new, send new job and load new
				loadNew = true
				out <- archiveJob{new: newJob}.Copy()
				continue
			}
		}

		debug.Log("    %q > %q, file %q removed", file1, file2, file1)
		// file has been removed, throw away old job and load new
		loadOld = true
	}
}

func (j archiveJob) Copy() pipe.Job {
	if !j.hasOld {
		return j.new
	}

	// handle files
	if isRegularFile(j.new.Info()) {
		debug.Log("   job %v is file", j.new.Path())

		// if type has changed, return new job directly
		if j.old.Node == nil {
			return j.new
		}

		// if file is newer, return the new job
		if j.old.Node.IsNewer(j.new.Fullpath(), j.new.Info()) {
			debug.Log("   job %v is newer", j.new.Path())
			return j.new
		}

		debug.Log("   job %v add old data", j.new.Path())
		// otherwise annotate job with old data
		e := j.new.(pipe.Entry)
		e.Node = j.old.Node
		return e
	}

	// dirs and other types are just returned
	return j.new
}

const saveIndexTime = 30 * time.Second

// saveIndexes regularly queries the master index for full indexes and saves them.
func (arch *Archiver) saveIndexes(wg *sync.WaitGroup, done <-chan struct{}) {
	defer wg.Done()

	ticker := time.NewTicker(saveIndexTime)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			debug.Log("saving full indexes")
			err := arch.repo.SaveFullIndex()
			if err != nil {
				debug.Log("save indexes returned an error: %v", err)
				fmt.Fprintf(os.Stderr, "error saving preliminary index: %v\n", err)
			}
		}
	}
}

// unique returns a slice that only contains unique strings.
func unique(items []string) []string {
	seen := make(map[string]struct{})
	for _, item := range items {
		seen[item] = struct{}{}
	}

	items = items[:0]
	for item := range seen {
		items = append(items, item)
	}
	return items
}

// baseNameSlice allows sorting paths by basename.
//
// Snapshots have contents sorted by basename, but we receive full paths.
// For the archivePipe to advance them in pairs, we traverse the given
// paths in the same order as the snapshot.
type baseNameSlice []string

func (p baseNameSlice) Len() int           { return len(p) }
func (p baseNameSlice) Less(i, j int) bool { return filepath.Base(p[i]) < filepath.Base(p[j]) }
func (p baseNameSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// Snapshot creates a snapshot of the given paths. If parentrestic.ID is set, this is
// used to compare the files to the ones archived at the time this snapshot was
// taken.
func (arch *Archiver) Snapshot(p *restic.Progress, paths, tags []string, parentID *restic.ID) (*restic.Snapshot, restic.ID, error) {
	paths = unique(paths)
	sort.Sort(baseNameSlice(paths))

	debug.Log("start for %v", paths)

	debug.RunHook("Archiver.Snapshot", nil)

	// signal the whole pipeline to stop
	done := make(chan struct{})
	var err error

	p.Start()
	defer p.Done()

	// create new snapshot
	sn, err := restic.NewSnapshot(paths, tags)
	if err != nil {
		return nil, restic.ID{}, err
	}
	sn.Excludes = arch.Excludes

	jobs := archivePipe{}

	// use parent snapshot (if some was given)
	if parentID != nil {
		sn.Parent = parentID

		// load parent snapshot
		parent, err := restic.LoadSnapshot(arch.repo, *parentID)
		if err != nil {
			return nil, restic.ID{}, err
		}

		// start walker on old tree
		ch := make(chan walk.TreeJob)
		go walk.Tree(arch.repo, *parent.Tree, done, ch)
		jobs.Old = ch
	} else {
		// use closed channel
		ch := make(chan walk.TreeJob)
		close(ch)
		jobs.Old = ch
	}

	// start walker
	pipeCh := make(chan pipe.Job)
	resCh := make(chan pipe.Result, 1)
	go func() {
		pipe.Walk(paths, arch.SelectFilter, done, pipeCh, resCh)
		debug.Log("pipe.Walk done")
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
		debug.Log("split done")
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

	// run index saver
	var wgIndexSaver sync.WaitGroup
	stopIndexSaver := make(chan struct{})
	wgIndexSaver.Add(1)
	go arch.saveIndexes(&wgIndexSaver, stopIndexSaver)

	// wait for all workers to terminate
	debug.Log("wait for workers")
	wg.Wait()

	// stop index saver
	close(stopIndexSaver)
	wgIndexSaver.Wait()

	debug.Log("workers terminated")

	// receive the top-level tree
	root := (<-resCh).(*restic.Node)
	debug.Log("root node received: %v", root.Subtree.Str())
	sn.Tree = root.Subtree

	// save snapshot
	id, err := arch.repo.SaveJSONUnpacked(restic.SnapshotFile, sn)
	if err != nil {
		return nil, restic.ID{}, err
	}

	debug.Log("saved snapshot %v", id.Str())

	// flush repository
	err = arch.repo.Flush()
	if err != nil {
		return nil, restic.ID{}, err
	}

	// save index
	err = arch.repo.SaveIndex()
	if err != nil {
		debug.Log("error saving index: %v", err)
		return nil, restic.ID{}, err
	}

	debug.Log("saved indexes")

	return sn, id, nil
}

func isRegularFile(fi os.FileInfo) bool {
	if fi == nil {
		return false
	}

	return fi.Mode()&(os.ModeType|os.ModeCharDevice) == 0
}

// Scan traverses the dirs to collect restic.Stat information while emitting progress
// information with p.
func Scan(dirs []string, filter pipe.SelectFunc, p *restic.Progress) (restic.Stat, error) {
	p.Start()
	defer p.Done()

	var stat restic.Stat

	for _, dir := range dirs {
		debug.Log("Start for %v", dir)
		err := fs.Walk(dir, func(str string, fi os.FileInfo, err error) error {
			// TODO: integrate error reporting
			if err != nil {
				fmt.Fprintf(os.Stderr, "error for %v: %v\n", str, err)
				return nil
			}
			if fi == nil {
				fmt.Fprintf(os.Stderr, "error for %v: FileInfo is nil\n", str)
				return nil
			}

			if !filter(str, fi) {
				debug.Log("path %v excluded", str)
				if fi.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			s := restic.Stat{}
			if fi.IsDir() {
				s.Dirs++
			} else {
				s.Files++

				if isRegularFile(fi) {
					s.Bytes += uint64(fi.Size())
				}
			}

			p.Report(s)
			stat.Add(s)

			// TODO: handle error?
			return nil
		})

		debug.Log("Done for %v, err: %v", dir, err)
		if err != nil {
			return restic.Stat{}, errors.Wrap(err, "fs.Walk")
		}
	}

	return stat, nil
}
