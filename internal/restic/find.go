package restic

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
)

// TreeLoader loads a tree from a repository.
type TreeLoader interface {
	LoadTree(context.Context, ID) (*Tree, error)
}

const numUsedBlobsWorkers = 4

// queue implements a concurrency-safe queue of IDs
// It can be accessed by the channel given with queue.Items()
type queue struct {
	q chan ID
	sync.WaitGroup
}

// NewQueue() gives a new empty queue
func NewQueue() *queue {
	return &queue{q: make(chan ID)}
}

// Add() adds the given IDs to the queue
func (q *queue) Add(ids IDs) {
	q.WaitGroup.Add(len(ids))
	go func() {
		for _, id := range ids {
			q.q <- id
		}
	}()
}

// Items() give a channel to listen for IDs in the queue.
// When an ID is processed queue.Done() should be called!
func (q queue) Items() <-chan ID {
	return q.q
}

// Wait() waits for all IDs to be processed and then closes the channel
// given by Items()
func (q *queue) Wait() {
	q.WaitGroup.Wait()
	close(q.q)
}

// FindUsedBlobs traverses the tree ID and adds all seen blobs (trees and data
// blobs) to the set blobs. Already seen tree blobs will not be visited again.
func FindUsedBlobs(ctx context.Context, repo TreeLoader, treeID ID, blobs BlobSet) error {
	var m sync.Mutex
	wg, ctx := errgroup.WithContext(ctx)
	queue := NewQueue()

	queue.Add(IDs{treeID})
	for i := 0; i < numUsedBlobsWorkers; i++ {
		wg.Go(func() error {
			return FindUsedBlobsWorker(queue, ctx, repo, blobs, &m)
		})
	}
	queue.Wait()
	return wg.Wait()
}

func FindUsedBlobsWorker(q *queue, ctx context.Context, repo TreeLoader, blobs BlobSet, m *sync.Mutex) error {
	for treeID := range q.Items() {
		h := BlobHandle{ID: treeID, Type: TreeBlob}
		m.Lock()
		if blobs.Has(h) {
			m.Unlock()
			q.Done()
			continue
		}
		blobs.Insert(h)
		m.Unlock()

		tree, err := repo.LoadTree(ctx, treeID)
		if err != nil {
			q.Done()
			return err
		}
		var subtrees IDs
		for _, node := range tree.Nodes {
			switch node.Type {
			case "file":
				for _, blob := range node.Content {
					m.Lock()
					blobs.Insert(BlobHandle{ID: blob, Type: DataBlob})
					m.Unlock()
				}
			case "dir":
				subtrees = append(subtrees, *node.Subtree)
			}
		}
		q.Add(subtrees)
		q.Done()
	}
	return nil
}
