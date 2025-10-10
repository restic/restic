package rechunker

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/ui/progress"
	"golang.org/x/sync/errgroup"
)

// data structure for debug trace
var debugNote = map[string]int{}
var debugNoteLock = sync.Mutex{}

type hashType = [32]byte
type fileInfo struct {
	restic.IDs
	hashval hashType
}
type srcChunkMsg struct {
	seqNum int
	blob   []byte
}
type newPipeMsg struct {
	seqNum int
	reader *io.PipeReader
}
type dstChunkMsg struct {
	seqNum int
	chunk  chunker.Chunk
}
type jumpMsg struct {
	seqNum  int
	blobIdx int
	offset  uint
}

type PackedBlobLoader interface {
	LoadBlob(ctx context.Context, t restic.BlobType, id restic.ID, buf []byte) ([]byte, error)
	LoadBlobsFromPack(ctx context.Context, packID restic.ID, blobs []restic.Blob, handleBlobFn func(blob restic.BlobHandle, buf []byte, err error) error) error
}

var ErrNewSequence = errors.New("new sequence")

type Rechunker struct {
	pol       chunker.Pol
	chunkDict *ChunkDict

	filesList    []restic.IDs
	blobSize     map[restic.ID]uint
	rechunkReady bool
	usePackCache bool

	// used if usePackCache == true
	blobToPack         map[restic.ID]restic.ID     // blob ID -> {blob length, pack ID}
	packToBlobs        map[restic.ID][]restic.Blob // pack ID -> list of blobs to be loaded from the pack
	sfPackToFiles      map[restic.ID][]fileInfo    // pack ID -> list of files{srcBlobIDs, hashOfIDs} that contains any blob in the pack (small files only)
	sfPackRequires     map[hashType]int            // hashOfIDs of srcBlobIDs -> number of packs until all blobs become ready in the cache (small files only)
	sfPackRequiresLock sync.Mutex

	// used in RewriteTree
	rechunkMap     map[hashType]restic.IDs // hashOfIDs of srcBlobIDs -> dstBlobIDs
	rewriteTreeMap map[restic.ID]restic.ID // original tree ID (in src repo) -> rewritten tree ID (in dst repo)
	rechunkMapLock sync.Mutex
}

func NewRechunker(pol chunker.Pol) *Rechunker {
	return &Rechunker{
		pol:            pol,
		chunkDict:      NewChunkDict(),
		rechunkMap:     map[hashType]restic.IDs{},
		rewriteTreeMap: map[restic.ID]restic.ID{},
	}
}

const SMALL_FILE_THRESHOLD = 50
const LARGE_FILE_THRESHOLD = 50

func (rc *Rechunker) reset() {
	rc.filesList = nil
	rc.blobSize = map[restic.ID]uint{}
	rc.rechunkReady = false

	rc.blobToPack = map[restic.ID]restic.ID{}
	rc.packToBlobs = map[restic.ID][]restic.Blob{}
	rc.sfPackToFiles = map[restic.ID][]fileInfo{}
	rc.sfPackRequires = map[hashType]int{}
}

func (rc *Rechunker) buildIndex(usePackCache bool, lookupBlobFn func(t restic.BlobType, id restic.ID) []restic.PackedBlob) error {
	// collect all required blobs
	allBlobs := restic.IDSet{}
	for _, file := range rc.filesList {
		for _, blob := range file {
			allBlobs.Insert(blob)
		}
	}

	// build blob lookup info
	for blob := range allBlobs {
		packs := lookupBlobFn(restic.DataBlob, blob)
		if len(packs) == 0 {
			return fmt.Errorf("can't find blob from source repo: %v", blob)
		}
		pb := packs[0]

		rc.blobSize[pb.Blob.ID] = pb.DataLength()
		if usePackCache {
			rc.blobToPack[pb.Blob.ID] = pb.PackID
			rc.packToBlobs[pb.PackID] = append(rc.packToBlobs[pb.PackID], pb.Blob)
		}
	}

	if !usePackCache { // nothing more to do
		return nil
	}

	// build file<->pack info for small files
	for _, file := range rc.filesList {
		if len(file) >= SMALL_FILE_THRESHOLD {
			continue
		}
		hashval := hashOfIDs(file)
		packSet := restic.IDSet{}
		for _, blob := range file {
			pack := rc.blobToPack[blob]
			packSet.Insert(pack)
		}
		rc.sfPackRequires[hashval] = len(packSet)
		for p := range packSet {
			rc.sfPackToFiles[p] = append(rc.sfPackToFiles[p], fileInfo{file, hashval})
		}
	}

	return nil
}

func (rc *Rechunker) Plan(ctx context.Context, srcRepo restic.Repository, rootTrees []restic.ID, usePackCache bool) error {
	rc.reset()

	visitedFiles := map[hashType]struct{}{}
	visitedTrees := restic.IDSet{}

	// skip previously processed files and trees
	for k := range rc.rechunkMap {
		visitedFiles[k] = struct{}{}
	}
	for k := range rc.rewriteTreeMap {
		visitedTrees.Insert(k)
	}

	wg, wgCtx := errgroup.WithContext(ctx)
	treeStream := data.StreamTrees(wgCtx, wg, srcRepo, rootTrees, func(id restic.ID) bool {
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
				if node.Type == data.NodeTypeFile {
					hashval := hashOfIDs(node.Content)
					if _, ok := visitedFiles[hashval]; ok {
						continue
					}
					visitedFiles[hashval] = struct{}{}

					rc.filesList = append(rc.filesList, node.Content)
				}
			}
		}
		return nil
	})
	err := wg.Wait()
	if err != nil {
		return err
	}

	err = rc.buildIndex(usePackCache, srcRepo.LookupBlob)
	if err != nil {
		return err
	}

	rc.usePackCache = usePackCache
	rc.rechunkReady = true

	return nil
}

func (rc *Rechunker) RechunkData(ctx context.Context, srcRepo PackedBlobLoader, dstRepo restic.BlobSaver, p *progress.Counter) error {
	if !rc.rechunkReady {
		return fmt.Errorf("Plan() must be run first before RechunkData()")
	}
	rc.rechunkReady = false

	wgMgr, wgMgrCtx := errgroup.WithContext(ctx)
	wgWkr, wgWkrCtx := errgroup.WithContext(ctx)
	numWorkers := runtime.GOMAXPROCS(0)

	// pack cache
	var cache *PackCache
	var blobGet func(blobID restic.ID, buf []byte) ([]byte, error)
	numDownloaders := min(numWorkers, 4)
	var priorityFilesList []restic.IDs
	var priorityFilesListLock sync.Mutex

	if rc.usePackCache {
		cache = NewPackCache(wgMgrCtx, wgMgr, rc.blobToPack, numDownloaders, func(packID restic.ID) (BlobsMap, error) {
			// downloadFn implementation
			blobData := BlobsMap{}
			blobs := rc.packToBlobs[packID]
			err := srcRepo.LoadBlobsFromPack(wgWkrCtx, packID, blobs,
				func(blob restic.BlobHandle, buf []byte, err error) error {
					if err != nil {
						return err
					}
					newBuf := make([]byte, len(buf))
					copy(newBuf, buf)
					blobData[blob.ID] = newBuf

					return nil
				})
			if err != nil {
				return BlobsMap{}, err
			}
			return blobData, nil
		}, func(packID restic.ID) {
			// debug trace
			debug.Log("Pack %v loaded", packID.Str())
			debugNoteLock.Lock()
			debugNote["load:"+packID.String()]++
			debugNoteLock.Unlock()

			// onPackReady implementation
			filesToUpdate := rc.sfPackToFiles[packID]
			var readyFiles []restic.IDs

			rc.sfPackRequiresLock.Lock()
			for _, file := range filesToUpdate {
				if rc.sfPackRequires[file.hashval] > 0 {
					rc.sfPackRequires[file.hashval]--
					if rc.sfPackRequires[file.hashval] == 0 {
						readyFiles = append(readyFiles, file.IDs)
					}
				}
			}
			rc.sfPackRequiresLock.Unlock()

			priorityFilesListLock.Lock()
			priorityFilesList = append(priorityFilesList, readyFiles...)
			priorityFilesListLock.Unlock()
		}, func(packID restic.ID) {
			// debug trace
			debug.Log("Pack %v evicted", packID.Str())
			debugNoteLock.Lock()
			debugNote["evict:"+packID.String()]++
			debugNoteLock.Unlock()

			// onPackEvict implementation
			filesToUpdate := rc.sfPackToFiles[packID]
			rc.sfPackRequiresLock.Lock()
			for _, file := range filesToUpdate {
				// files with sPackRequires==0 has already gone to priorityFilesList, so don't track them
				if rc.sfPackRequires[file.hashval] > 0 {
					rc.sfPackRequires[file.hashval]++
				}
			}
			rc.sfPackRequiresLock.Unlock()
		})

		// implement blobGet using blob cache
		blobGet = func(blobID restic.ID, buf []byte) ([]byte, error) {
			blob, ch := cache.Get(wgWkrCtx, wgWkr, blobID, buf)
			if blob == nil { // wait for blob to be downloaded
				select {
				case <-wgWkrCtx.Done():
					return nil, wgWkrCtx.Err()
				case blob = <-ch:
				}
			}
			return blob, nil
		}
	} else {
		blobGet = func(blobID restic.ID, buf []byte) ([]byte, error) {
			return srcRepo.LoadBlob(wgWkrCtx, restic.DataBlob, blobID, buf)
		}
	}

	// job dispatcher
	chDispatch := make(chan restic.IDs)
	if rc.usePackCache {
		wgMgr.Go(func() error {
			seenFiles := map[hashType]struct{}{}
			regularTrack := rc.filesList
			var fastTrack []restic.IDs

			for {
				if len(fastTrack) == 0 {
					priorityFilesListLock.Lock()
					if len(priorityFilesList) > 0 {
						fastTrack = priorityFilesList
						priorityFilesList = nil
					}
					priorityFilesListLock.Unlock()
				}

				if len(fastTrack) > 0 {
					file := fastTrack[0]
					fastTrack = fastTrack[1:]
					hashval := hashOfIDs(file)
					if _, ok := seenFiles[hashval]; ok {
						continue
					}
					seenFiles[hashval] = struct{}{}

					select {
					case <-wgMgrCtx.Done():
						return wgMgrCtx.Err()
					case chDispatch <- file:
					}
				} else if len(regularTrack) > 0 {
					file := regularTrack[0]
					regularTrack = regularTrack[1:]
					hashval := hashOfIDs(file)
					if _, ok := seenFiles[hashval]; ok {
						continue
					}
					seenFiles[hashval] = struct{}{}

					select {
					case <-wgMgrCtx.Done():
						return wgMgrCtx.Err()
					case chDispatch <- file:
					}
				} else { // no more jobs
					close(chDispatch)
					return nil
				}
			}
		})
	} else {
		wgMgr.Go(func() error {
			for _, file := range rc.filesList {
				select {
				case <-wgMgrCtx.Done():
					return wgMgrCtx.Err()
				case chDispatch <- file:
				}
			}
			close(chDispatch)
			return nil
		})
	}

	// rechunk workers
	bufferPool := make(chan []byte, 4*numWorkers)
	for range numWorkers {
		wgWkr.Go(func() error {
			chnker := chunker.New(nil, rc.pol)

			for {
				var srcBlobs restic.IDs
				var ok bool
				select {
				case <-wgWkrCtx.Done():
					return wgWkrCtx.Err()
				case srcBlobs, ok = <-chDispatch:
					if !ok { // all files finished and chan closed
						return nil
					}
				}
				dstBlobs := restic.IDs{}

				chSrcChunk := make(chan srcChunkMsg)
				chNewPipeMsg := make(chan newPipeMsg) // used only if useChunkDict
				chDstChunk := make(chan dstChunkMsg)
				chJumpMsg := make(chan jumpMsg, 1) // used only if useChunkDict
				r, w := io.Pipe()
				chnker.Reset(r, rc.pol)
				wg, wgCtx := errgroup.WithContext(wgWkrCtx)

				// prepare variables for chunkDict
				var blobPos []uint
				var seekBlobPos func(uint, int) (int, uint)
				var prefixPos uint
				var prefixIdx int
				useChunkDict := len(srcBlobs) != 0 && len(srcBlobs) >= LARGE_FILE_THRESHOLD
				if useChunkDict {
					// build blobPos (position of each blob in a file)
					blobPos = make([]uint, len(srcBlobs)+1)
					var offset uint
					for i, blob := range srcBlobs {
						offset += rc.blobSize[blob]
						blobPos[i+1] = offset
					}
					if blobPos[1] == 0 { // assertion
						panic("blobPos not computed correctly")
					}

					// define seekBlobPos
					seekBlobPos = func(pos uint, seekStartIdx int) (int, uint) {
						if pos < blobPos[seekStartIdx] { // invalid pos
							return -1, 0
						}
						i := seekStartIdx
						for i < len(srcBlobs) && pos >= blobPos[i+1] {
							i++
						}
						offset := pos - blobPos[i]

						return i, offset
					}

					// prefix match
					prefixBlobs, numFinishedBlobs, newOffset := rc.chunkDict.Match(srcBlobs, 0)
					if numFinishedBlobs > 0 {
						// debug trace
						debug.Log("ChunkDict match at %v (prefix): Skipping %d blobs", srcBlobs[0].Str(), numFinishedBlobs)
						debugNoteLock.Lock()
						debugNote["chunkdict_event"]++
						debugNote["chunkdict_blob_count"] += numFinishedBlobs
						debugNoteLock.Unlock()

						prefixIdx = numFinishedBlobs
						prefixPos = blobPos[numFinishedBlobs] + newOffset
						dstBlobs = prefixBlobs

						chJumpMsg <- jumpMsg{
							seqNum:  0,
							blobIdx: numFinishedBlobs,
							offset:  newOffset,
						}
					}
				}

				// run loader/iopipe/chunker/saver Goroutines per each file
				// loader: load original chunks one by one from source repo
				wg.Go(func() error {
					var seqNum int  // used only if useChunkDict
					var offset uint // used only if useChunkDict

				MainLoop:
					for i := 0; i < len(srcBlobs); i++ {
						if useChunkDict {
							select {
							case newPos := <-chJumpMsg:
								seqNum = newPos.seqNum
								i = newPos.blobIdx
								offset = newPos.offset
								if i >= len(srcBlobs) {
									break MainLoop
								}
							default:
							}
						}

						var buf []byte
						select {
						case buf = <-bufferPool:
						default:
							buf = make([]byte, chunker.MaxSize)
						}

						blob, err := blobGet(srcBlobs[i], buf)
						if err != nil {
							return err
						}
						if useChunkDict && offset != 0 {
							copy(blob, blob[offset:])
							blob = blob[:len(blob)-int(offset)]
							offset = 0
						}

						select {
						case <-wgCtx.Done():
							return wgCtx.Err()
						case chSrcChunk <- srcChunkMsg{seqNum, blob}:
						}
					}
					close(chSrcChunk)
					return nil
				})

				// iopipe: convert chunks into io.Reader stream
				wg.Go(func() error {
					var seqNum int // used only if useChunkDict

					for {
						var c srcChunkMsg
						var ok bool
						select {
						case <-wgCtx.Done():
							w.CloseWithError(wgCtx.Err())
							return wgCtx.Err()
						case c, ok = <-chSrcChunk:
							if !ok { // EOF
								err := w.Close()
								return err
							}
						}

						if useChunkDict && c.seqNum > seqNum {
							// new sequence
							seqNum = c.seqNum
							w.CloseWithError(ErrNewSequence)
							r, w = io.Pipe()
							select {
							case <-wgCtx.Done():
								return wgCtx.Err()
							case chNewPipeMsg <- newPipeMsg{seqNum, r}:
							}
						}

						buf := c.blob
						_, err := w.Write(buf)
						if err != nil {
							w.CloseWithError(err)
							return err
						}
						select {
						case bufferPool <- buf:
						default:
						}
					}

				})

				// chunker: rechunk filestream with destination repo's chunking parameter
				wg.Go(func() error {
					var seqNum int // used only if useChunkDict

					for {
						var buf []byte
						select {
						case buf = <-bufferPool:
						default:
							buf = make([]byte, chunker.MaxSize)
						}

						chunk, err := chnker.Next(buf)
						if err == io.EOF {
							select {
							case bufferPool <- buf:
							default:
							}
							close(chDstChunk)
							return nil
						}
						if useChunkDict && err == ErrNewSequence {
							select {
							case bufferPool <- buf:
							default:
							}
							select {
							case <-wgCtx.Done():
								return wgCtx.Err()
							case newPipe := <-chNewPipeMsg:
								seqNum = newPipe.seqNum
								r = newPipe.reader
								chnker.Reset(r, rc.pol)
							}
							continue
						}
						if err != nil {
							r.CloseWithError(err)
							return err
						}

						select {
						case <-wgCtx.Done():
							r.CloseWithError(wgCtx.Err())
							return wgCtx.Err()
						case chDstChunk <- dstChunkMsg{seqNum, chunk}:
						}
					}
				})

				// saver: save rechunked blobs into destination repo
				wg.Go(func() error {
					var seqNum int       // used only if useChunkDict
					currIdx := prefixIdx // used only if useChunkDict
					currPos := prefixPos // used only if useChunkDict

					for {
						var c dstChunkMsg
						var ok bool
						select {
						case <-wgCtx.Done():
							return wgCtx.Err()
						case c, ok = <-chDstChunk:
							if !ok { // EOF
								return nil
							}
						}

						if useChunkDict && c.seqNum < seqNum {
							// this chunk is skipped by previous chunkDict match, so just throw it away and wait for next chunk
							select {
							case bufferPool <- c.chunk.Data:
							default:
							}
							continue
						}

						blobData := c.chunk.Data
						dstBlobID, _, _, err := dstRepo.SaveBlob(ctx, restic.DataBlob, blobData, restic.ID{}, false)
						if err != nil {
							return err
						}

						if useChunkDict { // add chunk to chunkDict
							startOffset := currPos - blobPos[currIdx]
							endPos := currPos + c.chunk.Length
							endIdx, endOffset := seekBlobPos(endPos, currIdx)

							var chunkSrcBlobs []restic.ID
							if endIdx == len(srcBlobs) {
								chunkSrcBlobs = make([]restic.ID, endIdx-currIdx+1)
								n := copy(chunkSrcBlobs, srcBlobs[currIdx:endIdx]) // last element of chunkSrcBlobs is nullID
								if n != endIdx-currIdx {
									return fmt.Errorf("srcBlobs tail copy failed")
								}
							} else {
								chunkSrcBlobs = srcBlobs[currIdx : endIdx+1]
							}

							err := rc.chunkDict.Store(chunkSrcBlobs, startOffset, endOffset, dstBlobID)
							if err != nil {
								return err
							}

							currPos = endPos
							currIdx = endIdx
						}

						select {
						case bufferPool <- blobData:
						default:
						}
						dstBlobs = append(dstBlobs, dstBlobID)

						if useChunkDict { // match chunks from chunkDict and append them
							currOffset := currPos - blobPos[currIdx]
							matchedDstBlobs, numFinishedSrcBlobs, newOffset := rc.chunkDict.Match(srcBlobs[currIdx:], currOffset)
							if numFinishedSrcBlobs > 4 { // apply only when you can skip many blobs; otherwise, it would be better not to interrupt the pipeline
								// debug trace
								debug.Log("ChunkDict match at %v: Skipping %d blobs", srcBlobs[currIdx].Str(), numFinishedSrcBlobs)
								debugNoteLock.Lock()
								debugNote["chunkdict_event"]++
								debugNote["chunkdict_blob_count"] += numFinishedSrcBlobs
								debugNoteLock.Unlock()

								dstBlobs = append(dstBlobs, matchedDstBlobs...)

								currIdx += numFinishedSrcBlobs
								currPos = blobPos[currIdx] + newOffset

								seqNum++
								chJumpMsg <- jumpMsg{
									seqNum:  seqNum,
									blobIdx: currIdx,
									offset:  newOffset,
								}
							}
						}
					}
				})

				err := wg.Wait()
				if err != nil {
					return err
				}

				// register to rechunkMap
				hashval := hashOfIDs(srcBlobs)
				rc.rechunkMapLock.Lock()
				rc.rechunkMap[hashval] = dstBlobs
				rc.rechunkMapLock.Unlock()

				if p != nil {
					p.Add(1)
				}
			}
		})
	}

	// wait for rechunk workers to finish
	err := wgWkr.Wait()
	if err != nil {
		return err
	}
	// shutdown management workers
	if rc.usePackCache {
		cache.Close()
	}
	err = wgMgr.Wait()
	if err != nil {
		return err
	}

	// debug trace
	if rc.usePackCache {
		debug.Log("List of packs downloaded more than once:")
		numPackRedundant := 0
		redundantDownloadCount := 0
		for k := range debugNote {
			if strings.HasPrefix(k, "load:") && debugNote[k] > 1 {
				debug.Log("%v: Downloaded %d times, evicted %d times", k[5:15], debugNote[k], debugNote["evict:"+k[5:]])
				numPackRedundant++
				redundantDownloadCount += debugNote[k]
			}
		}
		debug.Log("[summary_packcache] Number of redundantly downloaded packs is %d, whose overall download count is %d", numPackRedundant, redundantDownloadCount)
	}
	debug.Log("[summary_chunkdict] ChunkDict match happend %d times, saving %d blob processings", debugNote["chunkdict_event"], debugNote["chunkdict_blob_count"])

	return err
}

func (rc *Rechunker) rewriteNode(node *data.Node) error {
	if node.Type != data.NodeTypeFile {
		return nil
	}

	hashval := hashOfIDs(node.Content)
	dstBlobs, ok := rc.rechunkMap[hashval]
	if !ok {
		return fmt.Errorf("can't find from rechunkBlobsMap: %v", node.Content.String())
	}
	node.Content = dstBlobs
	return nil
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

		if node.Type != data.NodeTypeDir {
			err = tb.AddNode(node)
			if err != nil {
				return restic.ID{}, err
			}
			continue
		}

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

	// Save new tree
	newTreeID, _, _, err := dstRepo.SaveBlob(ctx, restic.TreeBlob, tree, restic.ID{}, false)
	rc.rewriteTreeMap[nodeID] = newTreeID
	return newTreeID, err
}

func (rc *Rechunker) NumFilesToProcess() int {
	return len(rc.filesList)
}

func (rc *Rechunker) GetRewrittenTree(originalTree restic.ID) (restic.ID, error) {
	newID, ok := rc.rewriteTreeMap[originalTree]
	if !ok {
		return restic.ID{}, fmt.Errorf("rewritten tree does not exist for original tree %v", originalTree)
	}
	return newID, nil
}

func hashOfIDs(ids restic.IDs) hashType {
	c := make([]byte, 0, len(ids)*32)
	for _, id := range ids {
		c = append(c, id[:]...)
	}
	return sha256.Sum256(c)
}
