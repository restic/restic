package rechunker

import (
	"context"
	"errors"
	"io"

	"github.com/restic/chunker"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

func createBlobLoadCallback(ctx context.Context, c *BlobCache, idx *Index) blobLoadCallbackFn {
	if c == nil {
		return nil
	}

	return func(ids restic.IDs) error {
		var ignoresList restic.IDs
		idx.blobRemainingNeedsLock.Lock()
		for _, blob := range ids {
			idx.blobRemainingNeeds[blob]--
			if idx.blobRemainingNeeds[blob] == 0 {
				ignoresList = append(ignoresList, blob)
			}
		}
		idx.blobRemainingNeedsLock.Unlock()

		if len(ignoresList) > 0 {
			return c.Ignore(ctx, ignoresList)
		}

		return nil
	}
}

type ChunkedFile struct {
	restic.IDs
	hashval restic.ID
}

func prioritySelect(ctx context.Context, chFirst <-chan *ChunkedFile, chSecond <-chan *ChunkedFile) (file *ChunkedFile, ok bool, err error) {
	if chSecond != nil {
		// Firstly, try chFirst only. If chFirst is not ready now, wait for both chFirst and chSecond.
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case file, ok = <-chFirst:
			if ok {
				debug.Log("Selected file %v from chFirst (prioritized)", file.hashval.Str())
			}
		default:
			select {
			case <-ctx.Done():
				return nil, false, ctx.Err()
			case file, ok = <-chFirst:
				if ok {
					debug.Log("Selected file %v from chFirst", file.hashval.Str())
				}
			case file, ok = <-chSecond:
				if ok {
					debug.Log("Selected file %v from chSecond", file.hashval.Str())
				}
			}
		}
	} else {
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case file, ok = <-chFirst:
			if ok {
				debug.Log("Selected file %v", file.hashval.Str())
			}
		}
	}

	return file, ok, nil
}

type blob struct {
	buf       []byte
	stNum     int
	newStream bool
}
type fileStreamReader struct {
	*io.PipeReader
	stNum int
}
type chunk struct {
	chunker.Chunk
	stNum int
}
type fastForward struct {
	newStNum int
	blobIdx  int
	offset   uint
}
type fileResult struct {
	dstBlobs          restic.IDs
	addedToRepository uint64
}
type rechunkWorkerState struct {
	chunkDict  *ChunkDict
	idx        *Index
	chunker    *chunker.Chunker
	pol        chunker.Pol
	getBlob    getBlobFn
	saveBlob   saveBlobFn
	onBlobLoad blobLoadCallbackFn
}
type blobLoadCallbackFn func(ids restic.IDs) error

func startPipeline(ctx context.Context, wg *errgroup.Group, s *rechunkWorkerState, srcBlobs restic.IDs, out chan<- fileResult, bufferPool chan []byte, useChunkDict bool, p *Progress) {
	chBlob := make(chan blob)
	chStream := make(chan fileStreamReader)
	chChunk := make(chan chunk)

	// if useChunkDict == true, prepare ChunkDict helper and conduct prefix match
	var chFF chan fastForward
	var h *chunkDictHelper
	if useChunkDict {
		helper, ff, prefix := prepareChunkDict(srcBlobs, s.chunkDict, s.idx)
		chFF = make(chan fastForward, 1)
		h = helper
		if ff != nil {
			out <- fileResult{prefix, 0}
			chFF <- *ff
		}
	}

	// start pipeline components
	startBlobLoader(ctx, wg, srcBlobs, chBlob, s.getBlob, s.onBlobLoad, bufferPool, chFF)
	startFileStreamer(ctx, wg, chBlob, chStream, bufferPool)
	startChunker(ctx, wg, s.chunker, s.pol, chStream, chChunk, bufferPool)
	startBlobSaver(ctx, wg, chChunk, out, s.saveBlob, bufferPool, chFF, h, p)
}

func prepareChunkDict(srcBlobs restic.IDs, d *ChunkDict, idx *Index) (h *chunkDictHelper, ff *fastForward, prefix restic.IDs) {
	// build blobPos (position of each blob in a file)
	blobPos := make([]uint, len(srcBlobs)+1)
	var offset uint
	for i, blob := range srcBlobs {
		offset += idx.blobSize[blob]
		blobPos[i+1] = offset
	}
	if blobPos[1] == 0 { // assertion
		panic("blobPos not computed correctly")
	}

	// define seekBlobPos
	seekBlobPos := func(pos uint, seekStartIdx int) (int, uint) {
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
	prefix, numFinishedBlobs, newOffset := d.Match(srcBlobs, 0)
	var prefixIdx int
	var prefixPos uint
	if numFinishedBlobs > 0 {
		// debugNote: record chunkdict prefix match event
		debug.Log("ChunkDict prefix match at %v: Skipping %d blobs", srcBlobs[0].Str(), numFinishedBlobs)
		debugNote.AddMap(map[string]int{"chunkdict_event": 1, "chunkdict_blob_count": numFinishedBlobs})

		prefixIdx = numFinishedBlobs
		prefixPos = blobPos[prefixIdx] + newOffset

		ff = &fastForward{
			newStNum: 0,
			blobIdx:  prefixIdx,
			offset:   newOffset,
		}
	}

	h = &chunkDictHelper{
		d: d,

		srcBlobs:    srcBlobs,
		blobPos:     blobPos,
		seekBlobPos: seekBlobPos,

		currOffset: prefixPos,
		currIdx:    prefixIdx,
	}

	return h, ff, prefix
}

func startBlobLoader(ctx context.Context, wg *errgroup.Group, srcBlobs restic.IDs, out chan<- blob, getBlob getBlobFn, onBlobLoad blobLoadCallbackFn, bufferPool chan []byte, ff <-chan fastForward) {
	// loader: load file chunks sequentially, with possible fast-forward (blob skipping)
	wg.Go(func() error {
		var stNum int
		var offset uint
		var newStream bool

	MainLoop:
		for i := 0; i < len(srcBlobs); i++ {
			if ff != nil {
				// if fast-forward is enabled: check if a fast-forward request has arrived
				select {
				case ffPos := <-ff:
					debug.Log("Received FastForward; fast-forwarding %v blobs (stNum: %v)", ffPos.blobIdx-i, ffPos.newStNum)
					newStream = true
					if onBlobLoad != nil {
						if err := onBlobLoad(srcBlobs[i:ffPos.blobIdx]); err != nil {
							return err
						}
					}
					stNum = ffPos.newStNum
					i = ffPos.blobIdx
					offset = ffPos.offset
					if i >= len(srcBlobs) { // implies EOF
						break MainLoop
					}
				default:
					newStream = false
				}
			}

			// bring buffer from bufferPool
			var buf []byte
			select {
			case buf = <-bufferPool:
			default:
				debug.Log("Allocating a new buffer")
				buf = make([]byte, 0, chunker.MaxSize)
			}

			// get chunk data (may take a while)
			buf, err := getBlob(srcBlobs[i], buf, nil)
			if err != nil {
				return err
			}
			if ff != nil && offset != 0 {
				copy(buf, buf[offset:])
				buf = buf[:len(buf)-int(offset)]
				offset = 0
			}
			if onBlobLoad != nil {
				if err := onBlobLoad(srcBlobs[i : i+1]); err != nil {
					return err
				}
			}

			// send the chunk to iopipe
			select {
			case <-ctx.Done():
				return ctx.Err()
			case out <- blob{buf: buf, stNum: stNum, newStream: newStream}:
				debug.Log("Sent srcBlob %v to streamer", srcBlobs[i].Str())
			}
		}
		close(out)
		return nil
	})
}

var ErrNewStream = errors.New("new stream")

func startFileStreamer(ctx context.Context, wg *errgroup.Group, in <-chan blob, out chan<- fileStreamReader, bufferPool chan []byte) {
	wg.Go(func() error {
		r, w := io.Pipe()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- fileStreamReader{r, 0}: // send initial reader to chunker
		}

		for {
			// receive chunk from loader
			var b blob
			var ok bool
			select {
			case <-ctx.Done():
				w.CloseWithError(ctx.Err())
				return ctx.Err()
			case b, ok = <-in:
				if !ok { // EOF
					err := w.Close()
					return err
				}
			}

			// handle fast-forward if detected
			if b.newStream {
				debug.Log("Found new stNum (%v); creating new stream", b.stNum)
				w.CloseWithError(ErrNewStream)
				r, w = io.Pipe()
				select {
				case <-ctx.Done():
					return ctx.Err()
				case out <- fileStreamReader{r, b.stNum}: // send new reader to chunker
					debug.Log("New file stream reader sent")
				}
			}

			// stream-write to io.pipe
			buf := b.buf
			_, err := w.Write(buf)
			if err != nil {
				w.CloseWithError(err)
				return err
			}

			// recycle used buffer into bufferPool
			select {
			case bufferPool <- buf:
			default:
				debug.Log("bufferPool full; buffer discarded")
			}
		}
	})
}

func startChunker(ctx context.Context, wg *errgroup.Group, chnker *chunker.Chunker, pol chunker.Pol, in <-chan fileStreamReader, out chan<- chunk, bufferPool chan []byte) {
	wg.Go(func() error {
		var r fileStreamReader
		select {
		case <-ctx.Done():
			return ctx.Err()
		case r = <-in:
		}
		chnker.Reset(r, pol)

		for {
			// bring buffer from bufferPool
			var buf []byte
			select {
			case buf = <-bufferPool:
			default:
				buf = make([]byte, 0, chunker.MaxSize)
			}

			// rechunk with new parameter
			c, err := chnker.Next(buf)
			if err == io.EOF { // reached EOF; all done
				select {
				case bufferPool <- buf:
				default:
				}
				close(out)
				return nil
			}
			if err == ErrNewStream { // fast-forward occurred; replace fileStreamReader
				debug.Log("Received NewStream signal; preparing new reader")
				select {
				case bufferPool <- buf:
				default:
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case r = <-in:
					chnker.Reset(r, pol)
				}
				continue
			}
			if err != nil {
				r.CloseWithError(err)
				return err
			}

			// send chunk to blob saver
			select {
			case <-ctx.Done():
				r.CloseWithError(ctx.Err())
				return ctx.Err()
			case out <- chunk{c, r.stNum}:
				debug.Log("Sending a new chunk of size %v to blob saver", c.Length)
			}
		}
	})
}

func startBlobSaver(ctx context.Context, wg *errgroup.Group, in <-chan chunk, out chan<- fileResult, saveBlob saveBlobFn, bufferPool chan<- []byte, ff chan<- fastForward, h *chunkDictHelper, p *Progress) {
	wg.Go(func() error {
		var stNum int
		dstBlobs := restic.IDs{}
		var addedSize uint64

		for {
			// receive chunk from chunker
			var c chunk
			var ok bool
			select {
			case <-ctx.Done():
				return ctx.Err()
			case c, ok = <-in:
				if !ok { // EOF
					out <- fileResult{dstBlobs, addedSize}
					close(out)
					return nil
				}
			}

			if c.stNum < stNum {
				// just arrived chunk had been skipped by a chunkDict match,
				// so just flush it away and receive next chunk
				debug.Log("Chunk of obsolete stNum received; discarding.")
				select {
				case bufferPool <- c.Data:
				default:
					debug.Log("bufferPool full; buffer discarded")
				}
				continue
			}

			// save chunk to destination repo
			buf := c.Data
			dstBlobID, known, size, err := saveBlob(ctx, restic.DataBlob, buf, restic.ID{}, false)
			if err != nil {
				return err
			}
			if !known {
				addedSize += uint64(size)
			}
			debug.Log("Stored new dst chunk %v into dstRepo", dstBlobID.Str())

			if p != nil {
				p.AddBlob(uint64(c.Length))
			}

			// update chunk mapping to ChunkDict
			if h != nil {
				err = h.Update(dstBlobID, c.Length)
				if err != nil {
					return err
				}
			}

			// recycle used buffer into bufferPool
			select {
			case bufferPool <- buf:
			default:
				debug.Log("bufferPool full; buffer discarded")
			}
			dstBlobs = append(dstBlobs, dstBlobID)

			// retrieve chunk mapping from ChunkDict if there is a match
			if h != nil {
				matchedBlobs, length := h.Retrieve()

				if matchedBlobs != nil {
					dstBlobs = append(dstBlobs, matchedBlobs...)

					debug.Log("Sending FastForward with new stNum (%v->%v)", stNum, stNum+1)
					stNum++
					ff <- fastForward{
						newStNum: stNum,
						blobIdx:  h.currIdx,
						offset:   h.currOffset,
					}

					if p != nil {
						p.AddBlob(uint64(length))
					}
				}
			}
		}
	})
}
