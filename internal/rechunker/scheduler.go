package rechunker

import (
	"context"
	"fmt"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

type Scheduler struct {
	mu sync.Mutex

	idx Index

	regularCh  <-chan *ChunkedFile
	priorityCh <-chan *ChunkedFile

	regularList  []*ChunkedFile
	priorityList []*ChunkedFile

	prefixLookup         map[restic.ID][]*ChunkedFile // blob ID -> files that contain the blob as prefix
	remainingPrefixBlobs map[restic.ID]int            // file hashval -> remaining count until all its blobs ready

	remainingBlobUsage map[restic.ID]int // blob ID -> remaining blob usage until the end

	ignoreBlobsCB func(ids restic.IDs)

	push chan struct{}
	done chan struct{}
}

func NewScheduler(ctx context.Context, files []*ChunkedFile, idx Index, usePriority bool) *Scheduler {
	debug.Log(("Running NewScheduler()"))

	wg, ctx := errgroup.WithContext(ctx)
	filesContaining, blobsToPrepare, remainingBlobNeeds := createSchedulerState(files)

	if !usePriority {
		s := &Scheduler{
			idx:                  idx,
			regularList:          files,
			done:                 make(chan struct{}),
			prefixLookup:         filesContaining,
			remainingPrefixBlobs: blobsToPrepare,
			remainingBlobUsage:   remainingBlobNeeds,
		}
		s.createRegularCh(ctx, wg, nil)
		return s
	}

	s := &Scheduler{
		idx:                  idx,
		regularList:          files,
		push:                 make(chan struct{}, 1),
		done:                 make(chan struct{}),
		prefixLookup:         filesContaining,
		remainingPrefixBlobs: blobsToPrepare,
		remainingBlobUsage:   remainingBlobNeeds,
	}

	set := restic.IDSet{}
	mu := sync.Mutex{}
	visited := func(id restic.ID) bool {
		mu.Lock()
		visited := set.Has(id)
		if !visited {
			set.Insert(id)
		}
		mu.Unlock()
		return visited
	}

	s.createRegularCh(ctx, wg, visited)
	s.createPriorityCh(ctx, wg, visited)

	return s
}

const FILE_HEAD_LENGTH = 25

func createSchedulerState(files []*ChunkedFile) (map[restic.ID][]*ChunkedFile, map[restic.ID]int, map[restic.ID]int) {
	blobUsage := map[restic.ID]int{}
	prefixLookup := map[restic.ID][]*ChunkedFile{}
	numPrefixBlobs := map[restic.ID]int{}

	for _, file := range files {
		prefixLen := min(FILE_HEAD_LENGTH, len(file.IDs))
		prefixSet := restic.NewIDSet(file.IDs[:prefixLen]...)
		numPrefixBlobs[file.hashval] = len(prefixSet)
		for _, blob := range file.IDs {
			blobUsage[blob]++
		}
		for b := range prefixSet {
			prefixLookup[b] = append(prefixLookup[b], file)
		}
	}

	return prefixLookup, numPrefixBlobs, blobUsage
}

func (s *Scheduler) Next(ctx context.Context) (*ChunkedFile, bool, error) {
	file, from, err := PrioritySelect(ctx, s.priorityCh, s.regularCh)
	return file, from != 0, err
}

func (s *Scheduler) NextPriority(ctx context.Context) (*ChunkedFile, bool, error) {
	if s.priorityCh == nil {
		return nil, false, nil
	}
	file, from, err := PrioritySelect(ctx, s.priorityCh, nil)
	return file, from != 0, err
}

func (s *Scheduler) pushPriority(files []*ChunkedFile) {
	if s.priorityCh == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.priorityList = append(s.priorityList, files...)

	select {
	case s.push <- struct{}{}:
	default:
	}
}

func (s *Scheduler) popPriority() []*ChunkedFile {
	s.mu.Lock()
	defer s.mu.Unlock()

	l := s.priorityList
	s.priorityList = nil

	return l
}

func (s *Scheduler) createRegularCh(ctx context.Context, wg *errgroup.Group, visited func(id restic.ID) bool) {
	debug.Log("Running scheduler for regular channel")
	ch := make(chan *ChunkedFile)
	wg.Go(func() error {
		defer close(s.done)
		defer close(ch)

		for _, file := range s.regularList {
			if visited != nil && visited(file.hashval) {
				continue
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case ch <- file:
				debug.Log("Sent file %v through regular channel", file.hashval.Str())
			}
		}

		return nil
	})

	s.regularCh = ch
}

func (s *Scheduler) createPriorityCh(ctx context.Context, wg *errgroup.Group, visited func(id restic.ID) bool) {
	debug.Log("Running scheduler for priority channel")
	ch := make(chan *ChunkedFile)
	wg.Go(func() error {
		defer close(ch)

		var list []*ChunkedFile
		for {
			if len(list) == 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-s.push:
					list = s.popPriority()
					debug.Log("Detected priority files whose count is %v", len(list))
					continue
				case <-s.done:
					debug.Log("Closing scheduler for priority channel")
					return nil
				}
			}

			file := list[0]
			list = list[1:]

			if visited != nil && visited(file.hashval) {
				continue
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case ch <- file:
				debug.Log("Sent file %v through priority channel", file.hashval.Str())
			}
		}
	})

	s.priorityCh = ch
}

func (s *Scheduler) BlobReady(ids restic.IDs) {
	// when a new blob is ready, files containing that blob as their prefix
	// has their blobsToPrepare decreased by one.
	// The list of files whose blobs are all prepared is pushed to priority chan.

	if s.priorityCh == nil {
		// if there is no priority chan, it is of no meaning to track the state
		return
	}

	var readyFiles []*ChunkedFile

	s.mu.Lock()
	for _, id := range ids {
		for _, file := range s.prefixLookup[id] {
			n := s.remainingPrefixBlobs[file.hashval]
			if n > 0 {
				n--
				if n == 0 {
					readyFiles = append(readyFiles, file)
				}
				s.remainingPrefixBlobs[file.hashval] = n
			}
		}
	}
	s.mu.Unlock()

	if len(readyFiles) == 0 {
		return
	}

	s.pushPriority(readyFiles)

	if debugStats != nil {
		dAdds := map[string]int{}
		for _, id := range ids {
			dAdds["load:"+id.String()]++
		}
		debugStats.AddMap(dAdds)
	}
}

func (s *Scheduler) BlobUnready(ids restic.IDs) {
	// when a blob is evicted, files containing that blob as their prefix
	// has their blobsToPrepare increased by one. However, ignore files
	// once they have reached blobsToPrepare value zero; they are no longer tracked.

	if s.priorityCh == nil {
		// if there is no priority chan, it is of no meaning to track progress
		return
	}

	s.mu.Lock()
	for _, id := range ids {
		filesToUpdate := s.prefixLookup[id]
		for _, file := range filesToUpdate {
			// files with blobsToPrepare==0 is not tracked
			if s.remainingPrefixBlobs[file.hashval] > 0 {
				s.remainingPrefixBlobs[file.hashval]++
			}
		}
	}
	s.mu.Unlock()
}

func (s *Scheduler) SetIgnoreBlobsCallback(cb func(restic.IDs)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ignoreBlobsCB = cb
}

func (s *Scheduler) newCursor(blobs restic.IDs) cursor {
	if s == nil {
		return cursor{}
	}

	return cursor{
		blobs:    blobs,
		blobSize: s.idx.BlobSize,
	}
}

// updateCursor computes progress of cursor for a file, while inferring src blob consumption and using that info to track blob usage.
func (s *Scheduler) updateCursor(c cursor, bytesProcessed uint) (cursor, error) {
	start := c
	end, err := c.Advance(bytesProcessed)
	if err != nil {
		return cursor{}, err
	}

	if start.BlobIdx == end.BlobIdx {
		return end, nil
	}

	blobs := c.blobs[start.BlobIdx:end.BlobIdx]
	var obsolete restic.IDs
	s.mu.Lock()
	for _, b := range blobs {
		s.remainingBlobUsage[b]--
		if s.remainingBlobUsage[b] == 0 {
			obsolete = append(obsolete, b)
		}
	}
	s.mu.Unlock()

	if len(obsolete) == 0 {
		return end, nil
	}

	if s.ignoreBlobsCB != nil {
		s.ignoreBlobsCB(obsolete)
	}

	return end, nil
}

type cursor struct {
	blobs    restic.IDs
	BlobIdx  int
	Offset   uint
	blobSize func(restic.ID) uint
}

func (c cursor) Advance(numBytes uint) (cursor, error) {
	if c.blobs == nil {
		return cursor{}, nil
	}

	for c.BlobIdx < len(c.blobs) {
		blobSize := c.blobSize(c.blobs[c.BlobIdx])
		if blobSize == 0 {
			return cursor{}, fmt.Errorf("unknown blob %v", c.blobs[c.BlobIdx].Str())
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
		return cursor{}, fmt.Errorf("cursor out of range; %d bytes over end position", numBytes)
	}

	return c, nil
}

// PrioritySelect selects from two channels with priority; first channel first.
func PrioritySelect(ctx context.Context, first <-chan *ChunkedFile, second <-chan *ChunkedFile) (item *ChunkedFile, from int, err error) {
	// First, try to pull from channel 'first' only. If 'first' is not ready now, try both channels.
	select {
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	case i, ok := <-first:
		if ok {
			item = i
			from = 1
		}
	default:
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case i, ok := <-first:
			if ok {
				item = i
				from = 1
			}
		case i, ok := <-second:
			if ok {
				item = i
				from = 2
			}
		}
	}

	return item, from, nil
}
