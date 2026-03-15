package rechunker

import (
	"context"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

type Scheduler struct {
	mu sync.Mutex

	idx *Index

	regularCh  <-chan *ChunkedFile
	priorityCh <-chan *ChunkedFile

	regularList  []*ChunkedFile
	priorityList []*ChunkedFile

	filesContaining map[restic.ID][]*ChunkedFile
	blobsToPrepare  map[restic.ID]int

	remainingBlobNeeds map[restic.ID]int

	obsoleteBlobCB func(ids restic.IDs)

	push chan struct{}
	done chan struct{}
}

func NewScheduler(ctx context.Context, files []*ChunkedFile, idx *Index, usePriority bool) *Scheduler {
	debug.Log(("Running NewScheduler()"))

	wg, ctx := errgroup.WithContext(ctx)
	filesContaining, blobsToPrepare, remainingBlobNeeds := createSchedulerState(files)

	if !usePriority {
		s := &Scheduler{
			idx:                idx,
			regularList:        files,
			done:               make(chan struct{}),
			filesContaining:    filesContaining,
			blobsToPrepare:     blobsToPrepare,
			remainingBlobNeeds: remainingBlobNeeds,
		}
		s.createRegularCh(ctx, wg, nil)
		return s
	}

	s := &Scheduler{
		idx:                idx,
		regularList:        files,
		push:               make(chan struct{}, 1),
		done:               make(chan struct{}),
		filesContaining:    filesContaining,
		blobsToPrepare:     blobsToPrepare,
		remainingBlobNeeds: remainingBlobNeeds,
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
	blobCount := map[restic.ID]int{}
	filesContaining := map[restic.ID][]*ChunkedFile{}
	blobsToPrepare := map[restic.ID]int{}

	for _, file := range files {
		prefixLen := min(FILE_HEAD_LENGTH, len(file.IDs))
		blobSet := restic.NewIDSet(file.IDs[:prefixLen]...)
		blobsToPrepare[file.hashval] = len(blobSet)
		for _, blob := range file.IDs {
			blobCount[blob]++
		}
		for b := range blobSet {
			filesContaining[b] = append(filesContaining[b], file)
		}
	}

	return filesContaining, blobsToPrepare, blobCount
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

func (s *Scheduler) PushPriority(files []*ChunkedFile) {
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
		for _, file := range s.filesContaining[id] {
			n := s.blobsToPrepare[file.hashval]
			if n > 0 {
				n--
				if n == 0 {
					readyFiles = append(readyFiles, file)
				}
				s.blobsToPrepare[file.hashval] = n
			}
		}
	}
	s.mu.Unlock()

	if len(readyFiles) == 0 {
		return
	}

	s.PushPriority(readyFiles)

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
		filesToUpdate := s.filesContaining[id]
		for _, file := range filesToUpdate {
			// files with blobsToPrepare==0 is not tracked
			if s.blobsToPrepare[file.hashval] > 0 {
				s.blobsToPrepare[file.hashval]++
			}
		}
	}
	s.mu.Unlock()
}

func (s *Scheduler) SetObsoleteBlobCallback(cb func(restic.IDs)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.obsoleteBlobCB = cb
}

func (s *Scheduler) ReadProgress(cursor Cursor, bytesProcessed uint) (Cursor, error) {
	start := cursor
	end, err := AdvanceCursor(cursor, bytesProcessed, s.idx.BlobSize)
	if err != nil {
		return Cursor{}, err
	}

	if s.obsoleteBlobCB == nil {
		return end, nil
	}

	if start.BlobIdx == end.BlobIdx {
		return end, nil
	}

	blobs := cursor.blobs[start.BlobIdx:end.BlobIdx]
	var obsolete restic.IDs
	s.mu.Lock()
	for _, b := range blobs {
		s.remainingBlobNeeds[b]--
		if s.remainingBlobNeeds[b] == 0 {
			obsolete = append(obsolete, b)
		}
	}
	s.mu.Unlock()

	if len(obsolete) == 0 {
		return end, nil
	}

	if s.obsoleteBlobCB != nil {
		s.obsoleteBlobCB(obsolete)
	}
	return end, nil
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
