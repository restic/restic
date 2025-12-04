package rechunker

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

type Rechunker struct {
	cfg     Config
	tracker *eventTracker

	filesList    []*ChunkedFile
	totalSize    uint64
	rechunkReady bool

	idx *Index

	rechunkMap          map[restic.ID]restic.IDs // hashOfIDs of srcBlobIDs -> dstBlobIDs
	rechunkMapLock      sync.Mutex
	totalAddedToDstRepo atomic.Uint64
	rewriteTreeMap      map[restic.ID]restic.ID // original tree ID (in src repo) -> rewritten tree ID (in dst repo)
}

type Config struct {
	CacheSize          int
	SmallFileThreshold int // files less than the threshold will be prioritized when all blobs are ready in the cache
	Pol                chunker.Pol
}

// Index is immutable after Plan() returns.
type Index struct {
	BlobSize    map[restic.ID]uint
	BlobToPack  map[restic.ID]restic.ID     // blob ID -> {blob length, pack ID}
	PackToBlobs map[restic.ID][]restic.Blob // pack ID -> list of blobs to be loaded from the pack
}

func NewRechunker(cfg Config) *Rechunker {
	return &Rechunker{
		cfg:            cfg,
		rechunkMap:     map[restic.ID]restic.IDs{},
		rewriteTreeMap: map[restic.ID]restic.ID{},
	}
}

func (rc *Rechunker) reset() {
	rc.tracker = nil

	rc.filesList = nil
	rc.rechunkReady = false

	rc.idx = nil
}

func (rc *Rechunker) Plan(ctx context.Context, srcRepo restic.Repository, rootTrees []restic.ID) error {
	rc.reset()

	visitedFiles := restic.IDSet{}
	visitedTrees := restic.IDSet{}

	// skip previously processed files and trees
	for k := range rc.rechunkMap {
		visitedFiles.Insert(k)
	}
	for k := range rc.rewriteTreeMap {
		visitedTrees.Insert(k)
	}

	var err error
	debug.Log("Gathering distinct file Contents from target snapshots")
	rc.filesList, rc.totalSize, err = gatherFileContents(ctx, srcRepo, rootTrees, visitedFiles, visitedTrees)
	if err != nil {
		return err
	}

	debug.Log("Building the internal index for use in Rechunk()")
	rc.idx, rc.tracker, err = createIndex(rc.filesList, srcRepo.LookupBlob, rc.cfg)
	if err != nil {
		return err
	}

	debug.Log("Sorting the file list by their chunk counts (descending order)")
	slices.SortFunc(rc.filesList, func(a, b *ChunkedFile) int {
		return len(b.IDs) - len(a.IDs) // descending order
	})

	rc.rechunkReady = true

	return nil
}

func gatherFileContents(ctx context.Context, repo restic.Loader, rootTrees restic.IDs, visitedFiles restic.IDSet, visitedTrees restic.IDSet) (filesList []*ChunkedFile, totalSize uint64, err error) {
	wg, ctx := errgroup.WithContext(ctx)

	// create StreamTrees channel that streams through all subtrees in target snapshots
	treeStream := data.StreamTrees(ctx, wg, repo, rootTrees, func(id restic.ID) bool {
		visited := visitedTrees.Has(id)
		visitedTrees.Insert(id)
		return visited
	}, nil)

	// gather all distinct file Contents under trees
	wg.Go(func() error {
		for tree := range treeStream {
			if tree.Error != nil {
				return tree.Error
			}

			// check if the tree blob is unstable json
			buf, err := json.Marshal(tree.Tree)
			if err != nil {
				return err
			}
			buf = append(buf, '\n')
			if tree.ID != restic.Hash(buf) {
				return fmt.Errorf("can't run rechunk-copy, because the following tree can't be rewritten without losing information:\n%v", tree.ID.String())
			}

			for _, node := range tree.Nodes {
				// you only have to rechunk regular files; so skip other file types
				if node.Type == data.NodeTypeFile {
					hashval := HashOfIDs(node.Content)
					if visitedFiles.Has(hashval) {
						continue
					}
					visitedFiles.Insert(hashval)

					filesList = append(filesList, &ChunkedFile{
						node.Content,
						hashval,
					})
					totalSize += node.Size
				}
			}
		}
		return nil
	})
	err = wg.Wait()
	if err != nil {
		return nil, 0, err
	}
	return filesList, totalSize, nil
}

func createIndex(filesList []*ChunkedFile, lookupBlob func(t restic.BlobType, id restic.ID) []restic.PackedBlob, cfg Config) (*Index, *eventTracker, error) {
	// collect blob usage info
	blobCount := map[restic.ID]int{}
	for _, file := range filesList {
		for _, blob := range file.IDs {
			blobCount[blob]++
		}
	}

	// debugStats: record the number of blobs used
	if debugStats != nil {
		debugStats.Add("total_blob_count", len(blobCount))
	}

	// build blob lookup info
	blobSize := map[restic.ID]uint{}
	blobToPack := map[restic.ID]restic.ID{}
	packToBlobs := map[restic.ID][]restic.Blob{}
	for blob := range blobCount {
		packs := lookupBlob(restic.DataBlob, blob)
		if len(packs) == 0 {
			return nil, nil, fmt.Errorf("can't find blob from source repo: %v", blob)
		}
		pb := packs[0]

		blobSize[pb.Blob.ID] = pb.DataLength()
		blobToPack[pb.Blob.ID] = pb.PackID
		packToBlobs[pb.PackID] = append(packToBlobs[pb.PackID], pb.Blob)
	}

	idx := &Index{
		BlobSize:    blobSize,
		BlobToPack:  blobToPack,
		PackToBlobs: packToBlobs,
	}

	// build blob trace info for small files
	// if blob cache is enabled, Rechunker tracks small files' remaining blob count
	// until all blobs are available in the cache (rc.tracker.sfBlobRequires);
	// when the file has all its blobs ready, it is prioritized to be processed first.
	// this logic is handled by rc.priorityFilesHandler.
	sfBlobRequires := map[restic.ID]int{}
	sfBlobToFiles := map[restic.ID][]*ChunkedFile{}
	for _, file := range filesList {
		if file.Len() >= cfg.SmallFileThreshold {
			continue
		}
		blobSet := restic.NewIDSet(file.IDs...)
		sfBlobRequires[file.hashval] = len(blobSet)
		for b := range blobSet {
			sfBlobToFiles[b] = append(sfBlobToFiles[b], file)
		}
	}

	tracker := &eventTracker{
		idx:                idx,
		filesContaining:    sfBlobToFiles,
		blobsToPrepare:     sfBlobRequires,
		remainingBlobNeeds: blobCount,
	}

	return idx, tracker, nil
}

type Loader interface {
	restic.BlobLoader
	LoadBlobsFromPack(context.Context, restic.ID, []restic.Blob, func(restic.BlobHandle, []byte, error) error) error
	Connections() uint
}

func (rc *Rechunker) Rechunk(ctx context.Context, srcRepo Loader, dstRepo restic.WithBlobUploader, p *Progress) error {
	if !rc.rechunkReady {
		return fmt.Errorf("Plan() must be run first before RechunkData()")
	}
	rc.rechunkReady = false

	debug.Log("Rechunk start.")
	defer debug.Log("Rechunk done.")

	numWorkers := min(runtime.GOMAXPROCS(0), int(srcRepo.Connections()))
	numDownloaders := numWorkers
	debug.Log("srcRepo.Connections(): %v", srcRepo.Connections())

	// Phase 1: Setup Infrastructure

	// start blob cache
	var downloader restic.BlobLoader
	var cache *BlobCache
	if rc.cfg.CacheSize > 0 {
		downloader, cache = rc.setupCache(ctx, srcRepo, numDownloaders)
		defer cache.Close()
	} else {
		downloader = srcRepo
	}

	// start dispatcher
	dispatcher := rc.setupDispatcher(ctx)
	defer dispatcher.Close()

	// Phase 2: Run Workers
	bufferPool := NewBufferPool(2 * (numWorkers + 1))
	err := dstRepo.WithBlobUploader(ctx, func(ctx context.Context, uploader restic.BlobSaver) error {
		debug.Log("Starting uploader")
		defer debug.Log("Closing uploader")

		wg, ctx := errgroup.WithContext(ctx)
		rc.runWorkers(ctx, wg, numWorkers, downloader, uploader, dispatcher.Next, bufferPool, p)
		rc.runWorkers(ctx, wg, 1, downloader, uploader, dispatcher.NextPriority, bufferPool, p)

		return wg.Wait()
	})
	if err != nil {
		return err
	}

	debugPrintRechunkReport(rc)

	return nil
}

func (rc *Rechunker) setupCache(ctx context.Context, srcRepo PackLoader, numDownloaders int) (repo restic.BlobLoader, cache *BlobCache) {
	debug.Log("Creating blob cache: cacheSize %v", rc.cfg.CacheSize)

	// wrap srcRepo with cache. Now repo's LoadBlob() method will be transparently mediated by blob cache
	repo, cache = WrapWithCache(ctx, srcRepo, rc.cfg.CacheSize, numDownloaders, rc.idx, rc.tracker.BlobReady, rc.tracker.BlobUnready)

	// register callback to ignore obsolete blobs
	rc.tracker.obsoleteBlobCB = cache.Ignore

	return repo, cache
}

func (rc *Rechunker) setupDispatcher(ctx context.Context) (dispatcher *Dispatcher) {
	debug.Log("Running file dispatcher")

	// If the blob cache is enabled, priority dispatch will be used.
	// With priority dispatch, (small) files with all their blobs ready in the cache are prioritized.
	// if the blob cache is disabled, dispatch order simply follows the filesList.
	if rc.cfg.CacheSize > 0 {
		dispatcher = NewDispatcher(ctx, rc.filesList, true)

		// register callback to push priority files
		rc.tracker.priorityCB = dispatcher.PushPriority
	} else {
		dispatcher = NewDispatcher(ctx, rc.filesList, false)
	}
	return dispatcher
}

func (rc *Rechunker) runWorkers(ctx context.Context, wg *errgroup.Group, numWorkers int,
	downloader restic.BlobLoader, uploader restic.BlobSaver, receiveJob func(context.Context) (*ChunkedFile, bool, error),
	bufferPool *BufferPool, p *Progress) {
	for range numWorkers {
		wg.Go(func() error {
			debug.Log("Starting worker")
			worker := NewWorker(
				rc.cfg.Pol,
				downloader,
				uploader,
				bufferPool,
				rc.tracker.ReadProgress,
			)

			for {
				debug.Log("receiving job")
				file, ok, err := receiveJob(ctx)
				if err != nil {
					return err
				}
				if !ok {
					return nil
				}

				debug.Log("Starting file %v", file.hashval.Str())
				result, err := worker.RunFile(ctx, file.IDs, p)
				if err != nil {
					return err
				}
				debug.Log("Finished file %v", file.hashval.Str())
				if p != nil {
					p.AddFile(1)
				}

				rc.totalAddedToDstRepo.Add(result.addedToRepository)
				rc.rechunkMapLock.Lock()
				rc.rechunkMap[file.hashval] = result.dstBlobs
				rc.rechunkMapLock.Unlock()
			}
		})
	}
}

func (rc *Rechunker) RewriteTree(ctx context.Context, srcRepo restic.BlobLoader, dstRepo restic.BlobSaver, nodeID restic.ID) (restic.ID, error) {
	// check if the identical tree has already been processed
	newID, ok := rc.rewriteTreeMap[nodeID]
	if ok {
		return newID, nil
	}

	curTree, err := data.LoadTree(ctx, srcRepo, nodeID)
	if err != nil {
		return restic.ID{}, err
	}

	tb := data.NewTreeJSONBuilder()
	for _, node := range curTree.Nodes {
		if ctx.Err() != nil {
			return restic.ID{}, ctx.Err()
		}

		err = rc.rewriteNode(node)
		if err != nil {
			return restic.ID{}, err
		}

		// if the node is non-directory node, add it to the tree
		if node.Type != data.NodeTypeDir {
			err = tb.AddNode(node)
			if err != nil {
				return restic.ID{}, err
			}
			continue
		}

		// if the node is directory node, rewrite it recursively
		subtree := *node.Subtree
		newID, err := rc.RewriteTree(ctx, srcRepo, dstRepo, subtree)
		if err != nil {
			return restic.ID{}, err
		}
		node.Subtree = &newID
		err = tb.AddNode(node)
		if err != nil {
			return restic.ID{}, err
		}
	}

	tree, err := tb.Finalize()
	if err != nil {
		return restic.ID{}, err
	}

	// save new tree to the destination repo
	newTreeID, known, size, err := dstRepo.SaveBlob(ctx, restic.TreeBlob, tree, restic.ID{}, false)
	if err != nil {
		return restic.ID{}, err
	}
	rc.rewriteTreeMap[nodeID] = newTreeID

	if !known {
		rc.totalAddedToDstRepo.Add(uint64(size))
	}

	return newTreeID, err
}

func (rc *Rechunker) rewriteNode(node *data.Node) error {
	if node.Type != data.NodeTypeFile {
		return nil
	}

	hashval := HashOfIDs(node.Content)
	dstBlobs, ok := rc.rechunkMap[hashval]
	if !ok {
		return fmt.Errorf("can't find from rechunkBlobsMap: %v", node.Content.String())
	}
	node.Content = dstBlobs
	return nil
}

func (rc *Rechunker) NumFiles() int {
	return len(rc.filesList)
}

func (rc *Rechunker) GetRewrittenTree(originalTree restic.ID) (restic.ID, error) {
	newID, ok := rc.rewriteTreeMap[originalTree]
	if !ok {
		return restic.ID{}, fmt.Errorf("rewritten tree does not exist for original tree %v", originalTree)
	}
	return newID, nil
}

func (rc *Rechunker) TotalSize() uint64 {
	return rc.totalSize
}

func (rc *Rechunker) PackCount() int {
	return len(rc.idx.PackToBlobs)
}

func (rc *Rechunker) TotalAddedToDstRepo() uint64 {
	return rc.totalAddedToDstRepo.Load()
}

func (idx *Index) AdvanceCursor(c Cursor, bytesProcessed uint) Cursor {
	if idx == nil {
		panic("call from nil index")
	}

	for c.BlobIdx < len(c.blobs) {
		r := idx.BlobSize[c.blobs[c.BlobIdx]] - c.Offset

		if bytesProcessed < r {
			c.Offset += bytesProcessed
			bytesProcessed = 0
			break
		}

		bytesProcessed -= r
		c.BlobIdx++
		c.Offset = 0
	}

	return c
}

func HashOfIDs(ids restic.IDs) restic.ID {
	c := make([]byte, 0, len(ids)*32)
	for _, id := range ids {
		c = append(c, id[:]...)
	}
	return sha256.Sum256(c)
}

type Cursor struct {
	blobs   restic.IDs
	BlobIdx int
	Offset  uint
}

type Interval struct {
	Start Cursor
	End   Cursor
}

type ChunkedFile struct {
	restic.IDs
	hashval restic.ID
}

type eventTracker struct {
	mu sync.Mutex

	idx *Index

	filesContaining map[restic.ID][]*ChunkedFile // blobID -> files containing that blob
	blobsToPrepare  map[restic.ID]int            // file hashval -> number of blobs until all blobs ready in the cache

	remainingBlobNeeds map[restic.ID]int // blobID -> remaining blob needs

	priorityCB     func(files []*ChunkedFile) bool
	obsoleteBlobCB func(ids restic.IDs)
}

func (t *eventTracker) BlobReady(ids restic.IDs) {
	// when a new blob is ready, (small) files containing that blob has
	// their blobsToPrepare decreased by one.
	// The list of files whose blobs are all prepared is returned.

	if t.priorityCB == nil {
		// if there is no callback, it is of no meaning to track the state
		return
	}

	var readyFiles []*ChunkedFile

	t.mu.Lock()
	for _, id := range ids {
		for _, file := range t.filesContaining[id] {
			n := t.blobsToPrepare[file.hashval]
			if n > 0 {
				n--
				if n == 0 {
					readyFiles = append(readyFiles, file)
				}
				t.blobsToPrepare[file.hashval] = n
			}
		}
	}
	t.mu.Unlock()

	if len(readyFiles) == 0 {
		return
	}

	if t.priorityCB != nil {
		_ = t.priorityCB(readyFiles)
	}

	// debugStats: trace blob load count
	if debugStats != nil {
		dAdds := map[string]int{}
		for _, id := range ids {
			dAdds["load:"+id.String()]++
		}
		debugStats.AddMap(dAdds)
	}
}

func (t *eventTracker) BlobUnready(ids restic.IDs) {
	// when a blob is evicted, (small) files containing that blob has
	// their blobsToPrepare increased by one. However, ignore files
	// once they have reached blobsToPrepare value zero; they are no longer tracked.

	if t.priorityCB == nil {
		// if there is no callback, it is of no meaning to track progress
		return
	}

	t.mu.Lock()
	for _, id := range ids {
		filesToUpdate := t.filesContaining[id]
		for _, file := range filesToUpdate {
			// files with blobsToPrepare==0 is not tracked
			if t.blobsToPrepare[file.hashval] > 0 {
				t.blobsToPrepare[file.hashval]++
			}
		}
	}
	t.mu.Unlock()
}

func (t *eventTracker) ReadProgress(cursor Cursor, bytesProcessed uint) Cursor {
	start, end := cursor, t.idx.AdvanceCursor(cursor, bytesProcessed)

	if t.obsoleteBlobCB == nil {
		// if there is no callback, it is of no meaning to track the state
		return end
	}

	if start.BlobIdx == end.BlobIdx { // nothing to do
		return end
	}

	blobs := cursor.blobs[start.BlobIdx:end.BlobIdx]
	var obsolete restic.IDs
	t.mu.Lock()
	for _, b := range blobs {
		t.remainingBlobNeeds[b]--
		if t.remainingBlobNeeds[b] == 0 {
			obsolete = append(obsolete, b)
		}
	}
	t.mu.Unlock()

	if len(obsolete) == 0 {
		return end
	}

	if t.obsoleteBlobCB != nil {
		t.obsoleteBlobCB(obsolete)
	}
	return end
}
