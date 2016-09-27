package walk

import (
	"fmt"
	"os"
	"path/filepath"
	"restic"
	"sync"

	"restic/debug"
)

// TreeJob is a job sent from the tree walker.
type TreeJob struct {
	Path  string
	Error error

	Node *restic.Node
	Tree *restic.Tree
}

// TreeWalker traverses a tree in the repository depth-first and sends a job
// for each item (file or dir) that it encounters.
type TreeWalker struct {
	ch  chan<- loadTreeJob
	out chan<- TreeJob
}

// NewTreeWalker uses ch to load trees from the repository and sends jobs to
// out.
func NewTreeWalker(ch chan<- loadTreeJob, out chan<- TreeJob) *TreeWalker {
	return &TreeWalker{ch: ch, out: out}
}

// Walk starts walking the tree given by id. When the channel done is closed,
// processing stops.
func (tw *TreeWalker) Walk(path string, id restic.ID, done chan struct{}) {
	debug.Log("starting on tree %v for %v", id.Str(), path)
	defer debug.Log("done walking tree %v for %v", id.Str(), path)

	resCh := make(chan loadTreeResult, 1)
	tw.ch <- loadTreeJob{
		id:  id,
		res: resCh,
	}

	res := <-resCh
	if res.err != nil {
		select {
		case tw.out <- TreeJob{Path: path, Error: res.err}:
		case <-done:
			return
		}
		return
	}

	tw.walk(path, res.tree, done)

	select {
	case tw.out <- TreeJob{Path: path, Tree: res.tree}:
	case <-done:
		return
	}
}

func (tw *TreeWalker) walk(path string, tree *restic.Tree, done chan struct{}) {
	debug.Log("start on %q", path)
	defer debug.Log("done for %q", path)

	debug.Log("tree %#v", tree)

	// load all subtrees in parallel
	results := make([]<-chan loadTreeResult, len(tree.Nodes))
	for i, node := range tree.Nodes {
		if node.Type == "dir" {
			resCh := make(chan loadTreeResult, 1)
			tw.ch <- loadTreeJob{
				id:  *node.Subtree,
				res: resCh,
			}

			results[i] = resCh
		}
	}

	for i, node := range tree.Nodes {
		p := filepath.Join(path, node.Name)
		var job TreeJob

		if node.Type == "dir" {
			if results[i] == nil {
				panic("result chan should not be nil")
			}

			res := <-results[i]
			if res.err == nil {
				tw.walk(p, res.tree, done)
			} else {
				fmt.Fprintf(os.Stderr, "error loading tree: %v\n", res.err)
			}

			job = TreeJob{Path: p, Tree: res.tree, Error: res.err}
		} else {
			job = TreeJob{Path: p, Node: node}
		}

		select {
		case tw.out <- job:
		case <-done:
			return
		}
	}
}

type loadTreeResult struct {
	tree *restic.Tree
	err  error
}

type loadTreeJob struct {
	id  restic.ID
	res chan<- loadTreeResult
}

type treeLoader func(restic.ID) (*restic.Tree, error)

func loadTreeWorker(wg *sync.WaitGroup, in <-chan loadTreeJob, load treeLoader, done <-chan struct{}) {
	debug.Log("start")
	defer debug.Log("exit")
	defer wg.Done()

	for {
		select {
		case <-done:
			debug.Log("done channel closed")
			return
		case job, ok := <-in:
			if !ok {
				debug.Log("input channel closed, exiting")
				return
			}

			debug.Log("received job to load tree %v", job.id.Str())
			tree, err := load(job.id)

			debug.Log("tree %v loaded, error %v", job.id.Str(), err)

			select {
			case job.res <- loadTreeResult{tree, err}:
				debug.Log("job result sent")
			case <-done:
				debug.Log("done channel closed before result could be sent")
				return
			}
		}
	}
}

// TreeLoader loads tree objects.
type TreeLoader interface {
	LoadTree(restic.ID) (*restic.Tree, error)
}

const loadTreeWorkers = 10

// Tree walks the tree specified by id recursively and sends a job for each
// file and directory it finds. When the channel done is closed, processing
// stops.
func Tree(repo TreeLoader, id restic.ID, done chan struct{}, jobCh chan<- TreeJob) {
	debug.Log("start on %v, start workers", id.Str())

	load := func(id restic.ID) (*restic.Tree, error) {
		tree, err := repo.LoadTree(id)
		if err != nil {
			return nil, err
		}
		return tree, nil
	}

	ch := make(chan loadTreeJob)

	var wg sync.WaitGroup
	for i := 0; i < loadTreeWorkers; i++ {
		wg.Add(1)
		go loadTreeWorker(&wg, ch, load, done)
	}

	tw := NewTreeWalker(ch, jobCh)
	tw.Walk("", id, done)
	close(jobCh)

	close(ch)
	wg.Wait()

	debug.Log("done")
}
