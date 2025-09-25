package rechunker

import (
	"context"
	"fmt"
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

type link interface{} // union of {terminalLink, connectingLink}
type terminalLink struct {
	dstBlob restic.ID
	offset  uint
}
type connectingLink map[restic.ID]link
type linkIndex map[uint]link
type chunkDict map[restic.ID]linkIndex

type ChunkDict struct {
	dict chunkDict
	lock sync.RWMutex
}

func NewChunkDict() *ChunkDict {
	return &ChunkDict{
		dict: chunkDict{},
	}
}

func (cd *ChunkDict) Match(srcBlobs restic.IDs, startOffset uint) (dstBlobs restic.IDs, numFinishedBlobs int, newOffset uint) {
	if len(srcBlobs) == 0 { // nothing to return
		return
	}

	cd.lock.RLock()
	defer cd.lock.RUnlock()

	lnk, ok := cd.dict[srcBlobs[0]][startOffset]
	if !ok { // dict entry not found
		return
	}

	currentConsumedBlobs := 0
	for {
		switch v := lnk.(type) {
		case terminalLink:
			dstBlobs = append(dstBlobs, v.dstBlob)
			newOffset = v.offset
			numFinishedBlobs += currentConsumedBlobs
			currentConsumedBlobs = 0

			if len(srcBlobs) == 0 { // EOF
				return
			}
			lnk, ok = cd.dict[srcBlobs[0]][newOffset]
			if !ok {
				return
			}
		case connectingLink:
			currentConsumedBlobs++
			srcBlobs = srcBlobs[1:]

			if len(srcBlobs) == 0 { // reached EOF
				var nullID restic.ID
				lnk, ok = v[nullID]
				if !ok {
					return
				}
				_ = lnk.(terminalLink)
			} else { // go on to next blob
				lnk, ok = v[srcBlobs[0]]
				if !ok {
					return
				}
			}
		default:
			panic("wrong type")
		}
	}
}

func (cd *ChunkDict) Store(srcBlobs restic.IDs, startOffset, endOffset uint, dstBlob restic.ID) error {
	if len(srcBlobs) == 0 {
		return fmt.Errorf("empty srcBlobs")
	}
	if len(srcBlobs) == 1 && startOffset > endOffset {
		return fmt.Errorf("wrong value. len(srcBlob)==1 and startOffset>endOffset")
	}

	cd.lock.Lock()
	defer cd.lock.Unlock()

	idx, ok := cd.dict[srcBlobs[0]]
	if !ok {
		cd.dict[srcBlobs[0]] = linkIndex{}
		idx = cd.dict[srcBlobs[0]]
	}

	// create link head
	numConnectingLink := len(srcBlobs) - 1
	singleTerminalLink := (numConnectingLink == 0)
	lnk, ok := idx[startOffset]
	if ok { // index exists; type assertion
		if singleTerminalLink {
			_ = lnk.(terminalLink)
			return nil // nothing to touch
		}
		_ = lnk.(connectingLink)
	} else { // index does not exist
		if singleTerminalLink {
			idx[startOffset] = terminalLink{
				dstBlob: dstBlob,
				offset:  endOffset,
			}
			return nil
		}
		idx[startOffset] = connectingLink{}
		lnk = idx[startOffset]
	}
	srcBlobs = srcBlobs[1:]

	// build remaining connectingLink chain
	for range numConnectingLink - 1 {
		c := lnk.(connectingLink)
		lnk, ok = c[srcBlobs[0]]
		if !ok {
			c[srcBlobs[0]] = connectingLink{}
			lnk = c[srcBlobs[0]]
		}
		srcBlobs = srcBlobs[1:]
	}

	// create terminalLink
	c := lnk.(connectingLink)
	lnk, ok = c[srcBlobs[0]]
	if ok { // found that entire chain existed!
		_ = lnk.(terminalLink)
	} else {
		c[srcBlobs[0]] = terminalLink{
			dstBlob: dstBlob,
			offset:  endOffset,
		}
	}

	return nil
}

//////////

const LRU_SIZE = 200

type PackLRU = *lru.Cache[restic.ID, []restic.ID]

type packedBlobData struct {
	data   []byte
	packID restic.ID
}
type BlobsMap = map[restic.ID][]byte

type PackCache struct {
	pcklru         PackLRU
	packDownloadCh chan restic.ID
	blobToPack     map[restic.ID]restic.ID

	blobs          map[restic.ID]packedBlobData
	packWaiter     map[restic.ID]chan struct{}
	blobsLock      sync.RWMutex
	packWaiterLock sync.Mutex

	closed bool
}

func NewPackCache(ctx context.Context, wg *errgroup.Group, blobToPack map[restic.ID]restic.ID, numDownloaders int,
	downloadFn func(packID restic.ID) (BlobsMap, error), onPackReady func(packID restic.ID), onPackEvict func(packID restic.ID)) *PackCache {
	pc := &PackCache{
		packDownloadCh: make(chan restic.ID),
		blobToPack:     blobToPack,
		blobs:          map[restic.ID]packedBlobData{},
		packWaiter:     map[restic.ID]chan struct{}{},
	}
	lru, err := lru.NewWithEvict(LRU_SIZE, func(k restic.ID, v []restic.ID) {
		pc.packWaiterLock.Lock()
		delete(pc.packWaiter, k)
		pc.packWaiterLock.Unlock()
		pc.blobsLock.Lock()
		for _, blob := range v {
			delete(pc.blobs, blob)
		}
		pc.blobsLock.Unlock()
		if onPackEvict != nil {
			onPackEvict(k)
		}
	})
	if err != nil {
		panic(err)
	}
	pc.pcklru = lru

	// start pack downloader
	for range numDownloaders {
		wg.Go(func() error {
			for {
				var packID restic.ID
				var ok bool
				select {
				case <-ctx.Done():
					return ctx.Err()
				case packID, ok = <-pc.packDownloadCh:
					if !ok { // job complete
						return nil
					}
				}

				if pc.pcklru.Contains(packID) {
					// pack already downloaded by the previous request
					continue
				}
				blobsMap, err := downloadFn(packID)
				if err != nil {
					return err
				}
				blobIDs := make([]restic.ID, 0, len(blobsMap))
				for id := range blobsMap {
					blobIDs = append(blobIDs, id)
				}
				pc.blobsLock.Lock()
				for id, data := range blobsMap {
					pc.blobs[id] = packedBlobData{
						data:   data,
						packID: packID,
					}
				}
				pc.blobsLock.Unlock()
				_ = pc.pcklru.Add(packID, blobIDs)
				if onPackReady != nil {
					onPackReady(packID)
				}
				pc.packWaiterLock.Lock()
				close(pc.packWaiter[packID])
				pc.packWaiterLock.Unlock()
			}
		})
	}

	return pc
}

func (pc *PackCache) Get(ctx context.Context, wg *errgroup.Group, id restic.ID, buf []byte) ([]byte, chan []byte) {
	pc.blobsLock.RLock()
	blob, ok := pc.blobs[id]
	pc.blobsLock.RUnlock()
	if ok { // when blob exists in cache: return that blob
		_, _ = pc.pcklru.Get(blob.packID) // update recency
		if cap(buf) < len(blob.data) {
			debug.Log("buffer has smaller capacity than chunk size. Something might be wrong!")
			buf = make([]byte, len(blob.data))
		}
		buf = buf[:len(blob.data)]
		copy(buf, blob.data)
		return buf, nil
	}

	// when blob does not exist in cache: return async ch and send corresponding packID to downloader
	ch := make(chan []byte, 1) // where the downloaded blob will be delivered
	wg.Go(func() error {
		packID := pc.blobToPack[id]
		pc.packWaiterLock.Lock()
		chWaiter, ok := pc.packWaiter[packID]
		if !ok {
			chWaiter = make(chan struct{})
			pc.packWaiter[packID] = chWaiter
		}
		pc.packWaiterLock.Unlock()
		if !ok {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case pc.packDownloadCh <- packID:
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-chWaiter:
		}
		pc.blobsLock.RLock()
		blob, ok = pc.blobs[id]
		pc.blobsLock.RUnlock()
		if !ok {
			return fmt.Errorf("blob entry missing right after pack download. Please report this error at https://github.com/restic/restic/issues/")
		}
		if cap(buf) < len(blob.data) {
			debug.Log("buffer has smaller capacity than chunk size. Something might be wrong!")
			buf = make([]byte, len(blob.data))
		}
		buf = buf[:len(blob.data)]
		copy(buf, blob.data)
		ch <- buf
		return nil
	})
	return nil, ch
}

func (pc *PackCache) Close() {
	if !pc.closed {
		close(pc.packDownloadCh)
		pc.closed = true
	}
}
