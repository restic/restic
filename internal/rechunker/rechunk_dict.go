package rechunker

import (
	"fmt"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
)

// link is union of {terminalLink, connectingLink}.
type link interface{}

// terminalLink stores chunk mapping target, i.e., (dstBlob, new_offset).
type terminalLink struct {
	dstBlob restic.ID
	offset  uint
}

// connectingLink connects between two srcBlob.
type connectingLink map[restic.ID]link

// linkTable stores records for a srcBlob's each starting offset.
type linkTable map[uint]link

// dictIndex is a mapping whose key is the starting srcBlob and
// the value is a linkTable for that srcBlob.
type dictIndex map[restic.ID]linkTable

type seekBlobPosFn func(pos uint, seekStartIdx int) (idx int, offset uint) // given pos in a file, find blob idx and offset

type ChunkDict struct {
	idx dictIndex
	mu  sync.RWMutex
}

func NewChunkDict() *ChunkDict {
	return &ChunkDict{
		idx: dictIndex{},
	}
}

func (cd *ChunkDict) Match(srcBlobs restic.IDs, startOffset uint) (dstBlobs restic.IDs, numFinishedBlobs int, newOffset uint) {
	if len(srcBlobs) == 0 { // nothing to return
		return
	}

	cd.mu.RLock()
	defer cd.mu.RUnlock()

	lnk, ok := cd.idx[srcBlobs[0]][startOffset]
	if !ok { // can't find entry from index
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
			lnk, ok = cd.idx[srcBlobs[0]][newOffset]
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

	cd.mu.Lock()
	defer cd.mu.Unlock()

	table, ok := cd.idx[srcBlobs[0]]
	if !ok { // can't find table from idx; make new one
		cd.idx[srcBlobs[0]] = linkTable{}
		table = cd.idx[srcBlobs[0]]
	}

	// create link head
	numConnectingLink := len(srcBlobs) - 1
	singleTerminalLink := (numConnectingLink == 0)
	lnk, ok := table[startOffset]
	if ok { // table record exists; type assertion
		if singleTerminalLink {
			_ = lnk.(terminalLink)
			return nil // nothing to touch
		}
		_ = lnk.(connectingLink)
	} else { // table record does not exist; make new one
		if singleTerminalLink {
			table[startOffset] = terminalLink{
				dstBlob: dstBlob,
				offset:  endOffset,
			}
			return nil
		}
		table[startOffset] = connectingLink{}
		lnk = table[startOffset]
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

// chunkDictHelper has variables and functions needed for use in rechunk workers with ChunkDict.
type chunkDictHelper struct {
	d *ChunkDict

	srcBlobs    restic.IDs
	blobPos     []uint        // file position of each blob's start
	seekBlobPos seekBlobPosFn // maps file position to blob position

	currIdx    int
	currOffset uint
}

func (h *chunkDictHelper) Update(id restic.ID, length uint) error {
	endPos := h.blobPos[h.currIdx] + h.currOffset + length
	endIdx, endOffset := h.seekBlobPos(endPos, h.currIdx)

	// slice srcBlobs which corresponds to current chunk into chunkSrcBlobs
	var chunkSrcBlobs restic.IDs
	if endIdx == len(h.srcBlobs) { // tail-of-file chunk
		// last element of chunkSrcBlobs should be nullID, which indicates EOF
		chunkSrcBlobs = make(restic.IDs, endIdx-h.currIdx+1)
		n := copy(chunkSrcBlobs, h.srcBlobs[h.currIdx:endIdx])
		if n != endIdx-h.currIdx {
			panic("srcBlobs slice copy error")
		}
	} else { // mid-file chunk
		chunkSrcBlobs = h.srcBlobs[h.currIdx : endIdx+1]
	}

	// store chunk mapping to ChunkDict
	err := h.d.Store(chunkSrcBlobs, h.currOffset, endOffset, id)
	if err != nil {
		return err
	}

	// update current position in a file
	h.currOffset = endOffset
	h.currIdx = endIdx

	return nil
}

func (h *chunkDictHelper) Retrieve() (matchedBlobs restic.IDs, length uint) {
	matchedDstBlobs, numFinishedSrcBlobs, newOffset := h.d.Match(h.srcBlobs[h.currIdx:], h.currOffset)
	if numFinishedSrcBlobs > 4 { // apply only when you can skip many blobs; otherwise, it would be better not to interrupt the pipeline
		// debug trace
		debug.Log("ChunkDict match at %v: Skipping %d blobs", h.srcBlobs[h.currIdx].Str(), numFinishedSrcBlobs)
		debugNote.AddMap(map[string]int{"chunkdict_event": 1, "chunkdict_blob_count": numFinishedSrcBlobs})

		// compute new idx and pos
		oldPos := h.blobPos[h.currIdx] + h.currOffset
		h.currIdx += numFinishedSrcBlobs
		h.currOffset = newOffset
		newPos := h.blobPos[h.currIdx] + h.currOffset
		length := newPos - oldPos

		return matchedDstBlobs, length
	}
	return nil, 0
}
