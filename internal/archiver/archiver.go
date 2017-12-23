package archiver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walk"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/pipe"

	"github.com/restic/chunker"
)

const (
	maxConcurrentBlobs = 32
	maxConcurrency     = 10
)

var archiverPrintWarnings = func(path string, fi os.FileInfo, err error) {
	fmt.Fprintf(os.Stderr, "warning for %v: %v", path, err)
}
var archiverAllowAllFiles = func(string, os.FileInfo) bool { return true }

// Archiver is used to backup a set of directories.
type Archiver struct {
	repo       restic.Repository
	knownBlobs struct {
		restic.IDSet
		sync.Mutex
	}

	blobToken chan struct{}

	Warn         func(dir string, fi os.FileInfo, err error)
	SelectFilter pipe.SelectFunc
	Excludes     []string

	WithAccessTime bool
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

	arch.Warn = archiverPrintWarnings
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

	if arch.repo.Index().Has(id, t) {
		return true
	}

	return false
}

// Save stores a blob read from rd in the repository.
func (arch *Archiver) Save(ctx context.Context, t restic.BlobType, data []byte, id restic.ID) error {
	debug.Log("Save(%v, %v)\n", t, id)

	if arch.isKnownBlob(id, restic.DataBlob) {
		debug.Log("blob %v is known\n", id)
		return nil
	}

	_, err := arch.repo.SaveBlob(ctx, t, data, id)
	if err != nil {
		debug.Log("Save(%v, %v): error %v\n", t, id, err)
		return err
	}

	debug.Log("Save(%v, %v): new blob\n", t, id)
	return nil
}

// SaveTreeJSON stores a tree in the repository.
func (arch *Archiver) SaveTreeJSON(ctx context.Context, tree *restic.Tree) (restic.ID, error) {
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

	return arch.repo.SaveBlob(ctx, restic.TreeBlob, data, id)
}

func (arch *Archiver) reloadFileIfChanged(node *restic.Node, file fs.File) (*restic.Node, error) {
	if !arch.WithAccessTime {
		node.AccessTime = node.ModTime
	}

	fi, err := file.Stat()
	if err != nil {
		return nil, errors.Wrap(err, "restic.Stat")
	}

	if fi.ModTime().Equal(node.ModTime) {
		return node, nil
	}

	arch.Warn(node.Path, fi, errors.New("file has changed"))

	node, err = restic.NodeFromFileInfo(node.Path, fi)
	if err != nil {
		debug.Log("restic.NodeFromFileInfo returned error for %v: %v", node.Path, err)
		arch.Warn(node.Path, fi, err)
	}

	if !arch.WithAccessTime {
		node.AccessTime = node.ModTime
	}

	return node, nil
}

type saveResult struct {
	id    restic.ID
	bytes uint64
}

func (arch *Archiver) saveChunk(ctx context.Context, chunk chunker.Chunk, p *restic.Progress, token struct{}, file fs.File, resultChannel chan<- saveResult) {
	defer freeBuf(chunk.Data)

	id := restic.Hash(chunk.Data)
	err := arch.Save(ctx, restic.DataBlob, chunk.Data, id)
	// TODO handle error
	if err != nil {
		debug.Log("Save(%v) failed: %v", id, err)
		fmt.Printf("\nerror while saving data to the repo: %+v\n", err)
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

		debug.Log("  adding blob %s, %d bytes", b.id, b.bytes)
	}

	if bytes != node.Size {
		fmt.Fprintf(os.Stderr, "warning for %v: expected %d bytes, saved %d bytes\n", node.Path, node.Size, bytes)
	}

	debug.Log("SaveFile(%q): %v blobs\n", node.Path, len(results))

	return nil
}

// SaveFile stores the content of the file on the backend as a Blob by calling
// Save for each chunk.
func (arch *Archiver) SaveFile(ctx context.Context, p *restic.Progress, node *restic.Node) (*restic.Node, error) {
	file, err := fs.Open(node.Path)
	if err != nil {
		return node, errors.Wrap(err, "Open")
	}
	defer file.Close()

	debug.RunHook("archiver.SaveFile", node.Path)

	node, err = arch.reloadFileIfChanged(node, file)
	if err != nil {
		return node, err
	}

	chnker := chunker.New(file, arch.repo.Config().ChunkerPolynomial)
	resultChannels := [](<-chan saveResult){}

	for {
		chunk, err := chnker.Next(getBuf())
		if errors.Cause(err) == io.EOF {
			break
		}

		if err != nil {
			return node, errors.Wrap(err, "chunker.Next")
		}

		resCh := make(chan saveResult, 1)
		go arch.saveChunk(ctx, chunk, p, <-arch.blobToken, file, resCh)
		resultChannels = append(resultChannels, resCh)
	}

	results, err := waitForResults(resultChannels)
	if err != nil {
		return node, err
	}
	err = updateNodeContent(node, results)

	return node, err
}

func (arch *Archiver) fileWorker(ctx context.Context, wg *sync.WaitGroup, p *restic.Progress, entCh <-chan pipe.Entry) {
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
				debug.Log("restic.NodeFromFileInfo returned error for %v: %v", node.Path, err)
				arch.Warn(e.Fullpath(), e.Info(), err)
			}

			if !arch.WithAccessTime {
				node.AccessTime = node.ModTime
			}

			// try to use old node, if present
			if e.Node != nil {
				debug.Log("   %v use old data", e.Path())

				oldNode := e.Node.(*restic.Node)
				// check if all content is still available in the repository
				contentMissing := false
				for _, blob := range oldNode.Content {
					if !arch.repo.Index().Has(blob, restic.DataBlob) {
						debug.Log("   %v not using old data, %v is missing", e.Path(), blob)
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
				debug.Log("   read and save %v", e.Path())
				node, err = arch.SaveFile(ctx, p, node)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error for %v: %v\n", node.Path, err)
					arch.Warn(e.Path(), nil, err)
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
		case <-ctx.Done():
			// pipeline was cancelled
			return
		}
	}
}

func (arch *Archiver) dirWorker(ctx context.Context, wg *sync.WaitGroup, p *restic.Progress, dirCh <-chan pipe.Dir) {
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

				if node.Type == "dir" {
					debug.Log("got tree node for %s: %v", node.Path, node.Subtree)

					if node.Subtree == nil {
						debug.Log("subtree is nil for node %v", node.Path)
						continue
					}

					if node.Subtree.IsNull() {
						panic("invalid null subtree restic.ID")
					}
				}

				// insert node into tree, resolve name collisions
				name := node.Name
				i := 0
				for {
					i++
					err := tree.Insert(node)
					if err == nil {
						break
					}

					newName := fmt.Sprintf("%v-%d", name, i)
					fmt.Fprintf(os.Stderr, "%v: name collision for %q, renaming to %q\n", filepath.Dir(node.Path), node.Name, newName)
					node.Name = newName
				}

			}

			node := &restic.Node{}

			if dir.Path() != "" && dir.Info() != nil {
				n, err := restic.NodeFromFileInfo(dir.Fullpath(), dir.Info())
				if err != nil {
					arch.Warn(dir.Path(), dir.Info(), err)
				}
				node = n

				if !arch.WithAccessTime {
					node.AccessTime = node.ModTime
				}
			}

			if err := dir.Error(); err != nil {
				node.Error = err.Error()
			}

			id, err := arch.SaveTreeJSON(ctx, tree)
			if err != nil {
				panic(err)
			}
			debug.Log("save tree for %s: %v", dir.Path(), id)
			if id.IsNull() {
				panic("invalid null subtree restic.ID return from SaveTreeJSON()")
			}

			node.Subtree = &id

			debug.Log("sending result to %v", dir.Result())

			dir.Result() <- node
			if dir.Path() != "" {
				p.Report(restic.Stat{Dirs: 1})
			}
		case <-ctx.Done():
			// pipeline was cancelled
			return
		}
	}
}

type archivePipe struct {
	Old <-chan walk.TreeJob
	New <-chan pipe.Job
}

func copyJobs(ctx context.Context, in <-chan pipe.Job, out chan<- pipe.Job) {
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
		case <-ctx.Done():
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

func (a *archivePipe) compare(ctx context.Context, out chan<- pipe.Job) {
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

				copyJobs(ctx, a.New, out)
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
func (arch *Archiver) saveIndexes(saveCtx, shutdownCtx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(saveIndexTime)
	defer ticker.Stop()

	for {
		select {
		case <-saveCtx.Done():
			return
		case <-shutdownCtx.Done():
			return
		case <-ticker.C:
			debug.Log("saving full indexes")
			err := arch.repo.SaveFullIndex(saveCtx)
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
func (arch *Archiver) Snapshot(ctx context.Context, p *restic.Progress, paths, tags []string, hostname string, parentID *restic.ID, time time.Time) (*restic.Snapshot, restic.ID, error) {
	paths = unique(paths)
	sort.Sort(baseNameSlice(paths))

	debug.Log("start for %v", paths)

	debug.RunHook("Archiver.Snapshot", nil)

	// signal the whole pipeline to stop
	var err error

	p.Start()
	defer p.Done()

	// create new snapshot
	sn, err := restic.NewSnapshot(paths, tags, hostname, time)
	if err != nil {
		return nil, restic.ID{}, err
	}
	sn.Excludes = arch.Excludes

	// make paths absolute
	for i, path := range paths {
		if p, err := filepath.Abs(path); err == nil {
			paths[i] = p
		}
	}

	jobs := archivePipe{}

	// use parent snapshot (if some was given)
	if parentID != nil {
		sn.Parent = parentID

		// load parent snapshot
		parent, err := restic.LoadSnapshot(ctx, arch.repo, *parentID)
		if err != nil {
			return nil, restic.ID{}, err
		}

		// start walker on old tree
		ch := make(chan walk.TreeJob)
		go walk.Tree(ctx, arch.repo, *parent.Tree, ch)
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
		pipe.Walk(ctx, paths, arch.SelectFilter, pipeCh, resCh)
		debug.Log("pipe.Walk done")
	}()
	jobs.New = pipeCh

	ch := make(chan pipe.Job)
	go jobs.compare(ctx, ch)

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
		go arch.fileWorker(ctx, &wg, p, entCh)
		go arch.dirWorker(ctx, &wg, p, dirCh)
	}

	// run index saver
	var wgIndexSaver sync.WaitGroup
	shutdownCtx, indexShutdown := context.WithCancel(ctx)
	wgIndexSaver.Add(1)
	go arch.saveIndexes(ctx, shutdownCtx, &wgIndexSaver)

	// wait for all workers to terminate
	debug.Log("wait for workers")
	wg.Wait()

	// stop index saver
	indexShutdown()
	wgIndexSaver.Wait()

	debug.Log("workers terminated")

	// flush repository
	err = arch.repo.Flush(ctx)
	if err != nil {
		return nil, restic.ID{}, err
	}

	// receive the top-level tree
	root := (<-resCh).(*restic.Node)
	debug.Log("root node received: %v", root.Subtree)
	sn.Tree = root.Subtree

	// load top-level tree again to see if it is empty
	toptree, err := arch.repo.LoadTree(ctx, *root.Subtree)
	if err != nil {
		return nil, restic.ID{}, err
	}

	if len(toptree.Nodes) == 0 {
		return nil, restic.ID{}, errors.Fatal("no files/dirs saved, refusing to create empty snapshot")
	}

	// save index
	err = arch.repo.SaveIndex(ctx)
	if err != nil {
		debug.Log("error saving index: %v", err)
		return nil, restic.ID{}, err
	}

	debug.Log("saved indexes")

	// save snapshot
	id, err := arch.repo.SaveJSONUnpacked(ctx, restic.SnapshotFile, sn)
	if err != nil {
		return nil, restic.ID{}, err
	}

	debug.Log("saved snapshot %v", id)

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
