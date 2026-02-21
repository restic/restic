package rechunker

import (
	"context"
	"sync"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	"golang.org/x/sync/errgroup"
)

type Dispatcher struct {
	mu sync.Mutex

	// job dispatch channel to workers
	regular  <-chan *ChunkedFile
	priority <-chan *ChunkedFile

	// files list for dispatch
	regularList  []*ChunkedFile
	priorityList []*ChunkedFile

	push chan struct{} // priority file notification
	done chan struct{} // end of regular channel notification
}

func NewDispatcher(ctx context.Context, files []*ChunkedFile, usePriority bool) *Dispatcher {
	debug.Log(("Running NewDispatcher()"))

	wg, ctx := errgroup.WithContext(ctx)

	if !usePriority {
		// this will be a regular dispatcher without priority dispatch
		d := &Dispatcher{
			regularList: files,
			done:        make(chan struct{}),
		}
		d.createRegularCh(ctx, wg, nil)
		return d
	}

	// below is setup for priority-aware dispatcher

	d := &Dispatcher{
		regularList: files,
		push:        make(chan struct{}, 1),
		done:        make(chan struct{}),
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

	d.createRegularCh(ctx, wg, visited)
	d.createPriorityCh(ctx, wg, visited)

	return d
}

func (d *Dispatcher) Next(ctx context.Context) (*ChunkedFile, bool, error) {
	file, from, err := PrioritySelect(ctx, d.priority, d.regular)
	return file, from != 0, err
}

func (d *Dispatcher) NextPriority(ctx context.Context) (*ChunkedFile, bool, error) {
	if d.priority == nil {
		return nil, false, nil
	}
	file, from, err := PrioritySelect(ctx, d.priority, nil)
	return file, from != 0, err
}

func (d *Dispatcher) PushPriority(files []*ChunkedFile) bool {
	if d.priority == nil {
		return false
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.priorityList = append(d.priorityList, files...)

	// notify push channel
	select {
	case d.push <- struct{}{}:
	default:
	}

	return true
}

func (d *Dispatcher) popPriority() []*ChunkedFile {
	d.mu.Lock()
	defer d.mu.Unlock()

	l := d.priorityList
	d.priorityList = nil

	return l
}

func (d *Dispatcher) createRegularCh(ctx context.Context, wg *errgroup.Group, visited func(id restic.ID) bool) {
	debug.Log("Running dispatcher for regular channel")
	ch := make(chan *ChunkedFile)
	wg.Go(func() error {
		defer close(d.done)
		defer close(ch)

		for _, file := range d.regularList {
			// check if the file was visited by another dispatcher;
			// if it was, skip the file.
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

	d.regular = ch
}

func (d *Dispatcher) createPriorityCh(ctx context.Context, wg *errgroup.Group, visited func(id restic.ID) bool) {
	debug.Log("Running dispatcher for priority channel")
	ch := make(chan *ChunkedFile)
	wg.Go(func() error {
		defer close(ch)

		var list []*ChunkedFile
		for {
			if len(list) == 0 {
				// wait for priority files notification or done signal
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-d.push:
					list = d.popPriority()
					debug.Log("Detected priority files whose count is %v", len(list))
					continue
				case <-d.done:
					debug.Log("Closing dispatcher for priority channel")
					return nil
				}
			}

			file := list[0]
			list = list[1:]

			// check if the file was handled by another channel;
			// if it was, skip the file.
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

	d.priority = ch
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
