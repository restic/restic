package rechunker

import (
	"context"
	"crypto/sha256"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
	"golang.org/x/sync/errgroup"
)

type Rechunker struct {
	cfg Config
	idx *Index

	filesList    []*ChunkedFile
	totalSize    uint64
	rechunkReady bool

	rechunkMap          map[restic.ID]restic.IDs // hashOfIDs of srcBlobIDs -> dstBlobIDs
	rechunkMapLock      sync.Mutex
	totalAddedToDstRepo atomic.Uint64
	rewriteTreeMap      map[restic.ID]restic.ID // original tree ID (in src repo) -> rewritten tree ID (in dst repo)
}

type Config struct {
	CacheSize int
	Pol       chunker.Pol
}

type ChunkedFile struct {
	restic.IDs
	hashval restic.ID
}

// Index is immutable after Plan() returns.
type Index struct {
	BlobSize    map[restic.ID]uint          // blob ID -> blob size
	BlobToPack  map[restic.ID]restic.ID     // blob ID -> pack ID
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
	rc.idx = nil

	rc.filesList = nil
	rc.rechunkReady = false
}

func (rc *Rechunker) Plan(ctx context.Context, srcRepo restic.Repository, rootTrees restic.IDs) error {
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
	rc.idx, err = createIndex(rc.filesList, srcRepo.LookupBlob)
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
	mu := sync.Mutex{}

	// Stream through all subtrees in target rootTrees and gather all distinct file Contents
	err = data.StreamTrees(ctx, repo, rootTrees, nil, func(id restic.ID) bool {
		visited := visitedTrees.Has(id)
		visitedTrees.Insert(id)
		return visited
	}, func(_ restic.ID, err error, nodes data.TreeNodeIterator) error {
		if err != nil {
			return err
		}

		for item := range nodes {
			if item.Error != nil {
				return item.Error
			}
			if item.Node == nil || item.Node.Type != data.NodeTypeFile {
				continue
			}

			hashval := HashOfIDs(item.Node.Content)

			mu.Lock()
			if visitedFiles.Has(hashval) {
				mu.Unlock()
				continue
			}
			visitedFiles.Insert(hashval)

			filesList = append(filesList, &ChunkedFile{
				item.Node.Content,
				hashval,
			})
			totalSize += item.Node.Size
			mu.Unlock()
		}

		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	return filesList, totalSize, nil
}

func createIndex(filesList []*ChunkedFile, lookupBlob func(t restic.BlobType, id restic.ID) []restic.PackedBlob) (*Index, error) {
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
			return nil, fmt.Errorf("can't find blob from source repo: %v", blob)
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

	return idx, nil
}

type Loader interface {
	restic.BlobLoader
	LoadBlobsFromPack(context.Context, restic.ID, []restic.Blob, func(restic.BlobHandle, []byte, error) error) error
	Connections() uint
}

func (rc *Rechunker) Rechunk(ctx context.Context, srcRepo Loader, dstRepo restic.WithBlobUploader, p *Progress) error {
	if !rc.rechunkReady {
		return fmt.Errorf("Plan() must be run first before Rechunk()")
	}
	rc.rechunkReady = false

	debug.Log("Rechunk start.")
	defer debug.Log("Rechunk done.")

	numWorkers := min(runtime.GOMAXPROCS(0), int(srcRepo.Connections()))
	numDownloaders := numWorkers
	debug.Log("srcRepo.Connections(): %v", srcRepo.Connections())

	// set up scheduler
	scheduler := rc.setupScheduler(ctx)

	// set up blob cache
	var downloader restic.BlobLoader
	var cache *BlobCache
	if rc.cfg.CacheSize > 0 {
		downloader, cache = rc.setupCache(ctx, srcRepo, scheduler, numDownloaders)
		defer cache.Close()
	} else {
		downloader = srcRepo
	}

	// run rechunk workers
	bufferPool := NewBufferPool(3 * (numWorkers + 1))
	err := dstRepo.WithBlobUploader(ctx, func(ctx context.Context, uploader restic.BlobSaverWithAsync) error {
		debug.Log("Starting uploader")
		defer debug.Log("Closing uploader")

		wg, ctx := errgroup.WithContext(ctx)
		rc.runWorkers(ctx, wg, numWorkers, downloader, uploader, scheduler.Next, scheduler.ReadProgress, bufferPool, p)
		rc.runWorkers(ctx, wg, 1, downloader, uploader, scheduler.NextPriority, scheduler.ReadProgress, bufferPool, p)

		return wg.Wait()
	})
	if err != nil {
		return err
	}

	debugPrintRechunkReport(rc)

	return nil
}

func (rc *Rechunker) setupScheduler(ctx context.Context) (scheduler *Scheduler) {
	debug.Log("Running file dispatcher")

	// If the blob cache is enabled, priority dispatch will be used.
	// With priority dispatch, (small) files with all their blobs ready in the cache are prioritized.
	// if the blob cache is disabled, dispatch order simply follows the filesList.
	if rc.cfg.CacheSize > 0 {
		scheduler = NewScheduler(ctx, rc.filesList, rc.idx, true)
	} else {
		scheduler = NewScheduler(ctx, rc.filesList, rc.idx, false)
	}
	return scheduler
}

func (rc *Rechunker) setupCache(ctx context.Context, srcRepo PackLoader, scheduler *Scheduler, numDownloaders int) (repo restic.BlobLoader, cache *BlobCache) {
	debug.Log("Creating blob cache: cacheSize %v", rc.cfg.CacheSize)

	// wrap srcRepo with cache. Now repo's LoadBlob() method will be transparently mediated by blob cache
	repo, cache = WrapWithCache(ctx, srcRepo, rc.cfg.CacheSize, numDownloaders, rc.idx, scheduler.BlobReady, scheduler.BlobUnready)

	// register cache.Ignore as scheduler's obsolete blob callback for early cache eviction
	scheduler.SetObsoleteBlobCallback(cache.Ignore)

	return repo, cache
}

func (rc *Rechunker) runWorkers(ctx context.Context, wg *errgroup.Group, numWorkers int,
	downloader restic.BlobLoader, uploader restic.BlobSaver, receiveJob func(context.Context) (*ChunkedFile, bool, error),
	cursorProgressor func(Cursor, uint) (Cursor, error), bufferPool *BufferPool, p *Progress) {
	for range numWorkers {
		wg.Go(func() error {
			debug.Log("Starting worker")
			worker := NewWorker(
				rc.cfg.Pol,
				downloader,
				uploader,
				bufferPool,
				cursorProgressor,
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

// wrapper type for BlobSaver where you can define custom SaveBlob()
type wrappedBlobSaver func(ctx context.Context, tpe restic.BlobType, buf []byte, id restic.ID, storeDuplicate bool) (newID restic.ID, known bool, sizeInRepo int, err error)

func (s wrappedBlobSaver) SaveBlob(ctx context.Context, tpe restic.BlobType, buf []byte, id restic.ID, storeDuplicate bool) (newID restic.ID, known bool, sizeInRepo int, err error) {
	return s(ctx, tpe, buf, id, storeDuplicate)
}

func (rc *Rechunker) RewriteTree(ctx context.Context, srcRepo restic.BlobLoader, dstRepo restic.BlobSaver, treeID restic.ID) (restic.ID, error) {
	// check if the identical tree has already been processed
	newID, ok := rc.rewriteTreeMap[treeID]
	if ok {
		return newID, nil
	}

	// wrap dstRepo so that total uploaded tree blobs size can be tracked
	saver := wrappedBlobSaver(func(ctx context.Context, tpe restic.BlobType, buf []byte, id restic.ID, storeDuplicate bool) (newID restic.ID, known bool, sizeInRepo int, err error) {
		newID, known, sizeInRepo, err = dstRepo.SaveBlob(ctx, tpe, buf, id, storeDuplicate)
		if err != nil {
			return
		}
		if !known {
			rc.totalAddedToDstRepo.Add(uint64(sizeInRepo))
		}
		return
	})

	// prepare rewriter that rewrites node.Content of regular files
	rewriter := walker.NewTreeRewriter(walker.RewriteOpts{
		RewriteNode: func(node *data.Node, _ string) *data.Node {
			if node == nil {
				return nil
			}
			if node.Type != data.NodeTypeFile {
				return node
			}

			hashval := HashOfIDs(node.Content)
			dstBlobs, ok := rc.rechunkMap[hashval]
			if !ok {
				panic(fmt.Errorf("can't find from rechunkBlobsMap: %v", node.Content.String()))
			}
			node.Content = dstBlobs
			return node
		},
	})

	newID, err := rewriter.RewriteTree(ctx, srcRepo, saver, "/", treeID)
	if err != nil {
		return restic.ID{}, err
	}

	rc.rewriteTreeMap[treeID] = newID

	return newID, err
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

func AdvanceCursor(c Cursor, numBytes uint, blobSizes map[restic.ID]uint) (Cursor, error) {
	for c.BlobIdx < len(c.blobs) {
		blobSize, ok := blobSizes[c.blobs[c.BlobIdx]]
		if !ok {
			return Cursor{}, fmt.Errorf("blob %v not in blobSizes", c.blobs[c.BlobIdx].Str())
		}
		r := blobSize - c.Offset

		if numBytes < r {
			c.Offset += numBytes
			numBytes = 0
			break
		}

		numBytes -= r
		c.BlobIdx++
		c.Offset = 0
	}

	if numBytes != 0 {
		return Cursor{}, fmt.Errorf("cursor out of range; %d bytes over end position", numBytes)
	}

	return c, nil
}
