package pipe_test

import (
	"flag"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/restic/restic/pipe"
)

var testWalkerPath = flag.String("test.walkerpath", ".", "pipeline walker testpath (default: .)")
var maxWorkers = flag.Int("test.workers", 100, "max concurrency (default: 100)")

func isFile(fi os.FileInfo) bool {
	return fi.Mode()&(os.ModeType|os.ModeCharDevice) == 0
}

type stats struct {
	dirs, files int
}

func statPath(path string) (stats, error) {
	var s stats

	// count files and directories with filepath.Walk()
	err := filepath.Walk(*testWalkerPath, func(p string, fi os.FileInfo, err error) error {
		if fi == nil {
			return err
		}

		if fi.IsDir() {
			s.dirs++
		} else {
			s.files++
		}

		return err
	})

	return s, err
}

func TestPipelineWalker(t *testing.T) {
	if *testWalkerPath == "" {
		t.Skipf("walkerpah not set, skipping TestPipelineWalker")
	}

	before, err := statPath(*testWalkerPath)
	ok(t, err)

	t.Logf("walking path %s with %d dirs, %d files", *testWalkerPath,
		before.dirs, before.files)

	after := stats{}
	m := sync.Mutex{}

	worker := func(wg *sync.WaitGroup, done <-chan struct{}, entCh <-chan pipe.Entry, dirCh <-chan pipe.Dir) {
		defer wg.Done()
		for {
			select {
			case e, ok := <-entCh:
				if !ok {
					// channel is closed
					return
				}

				m.Lock()
				after.files++
				m.Unlock()

				e.Result <- true

			case dir, ok := <-dirCh:
				if !ok {
					// channel is closed
					return
				}

				// wait for all content
				for _, ch := range dir.Entries {
					<-ch
				}

				m.Lock()
				after.dirs++
				m.Unlock()

				dir.Result <- true
			case <-done:
				// pipeline was cancelled
				return
			}
		}
	}

	var wg sync.WaitGroup
	done := make(chan struct{})
	entCh := make(chan pipe.Entry)
	dirCh := make(chan pipe.Dir)

	for i := 0; i < *maxWorkers; i++ {
		wg.Add(1)
		go worker(&wg, done, entCh, dirCh)
	}

	resCh, err := pipe.Walk(*testWalkerPath, done, entCh, dirCh)
	ok(t, err)

	// wait for all workers to terminate
	wg.Wait()

	// wait for top-level blob
	<-resCh

	t.Logf("walked path %s with %d dirs, %d files", *testWalkerPath,
		after.dirs, after.files)

	assert(t, before == after, "stats do not match, expected %v, got %v", before, after)
}

func BenchmarkPipelineWalker(b *testing.B) {
	if *testWalkerPath == "" {
		b.Skipf("walkerpah not set, skipping BenchPipelineWalker")
	}

	var max time.Duration
	m := sync.Mutex{}

	fileWorker := func(wg *sync.WaitGroup, done <-chan struct{}, ch <-chan pipe.Entry) {
		defer wg.Done()
		for {
			select {
			case e, ok := <-ch:
				if !ok {
					// channel is closed
					return
				}

				// simulate backup
				//time.Sleep(10 * time.Millisecond)

				e.Result <- true
			case <-done:
				// pipeline was cancelled
				return
			}
		}
	}

	dirWorker := func(wg *sync.WaitGroup, done <-chan struct{}, ch <-chan pipe.Dir) {
		defer wg.Done()
		for {
			select {
			case dir, ok := <-ch:
				if !ok {
					// channel is closed
					return
				}

				start := time.Now()

				// wait for all content
				for _, ch := range dir.Entries {
					<-ch
				}

				d := time.Since(start)
				m.Lock()
				if d > max {
					max = d
				}
				m.Unlock()

				dir.Result <- true
			case <-done:
				// pipeline was cancelled
				return
			}
		}
	}

	for i := 0; i < b.N; i++ {
		max = 0
		done := make(chan struct{})
		entCh := make(chan pipe.Entry, 200)
		dirCh := make(chan pipe.Dir, 200)

		var wg sync.WaitGroup
		b.Logf("starting %d workers", *maxWorkers)
		for i := 0; i < *maxWorkers; i++ {
			wg.Add(2)
			go dirWorker(&wg, done, dirCh)
			go fileWorker(&wg, done, entCh)
		}

		resCh, err := pipe.Walk(*testWalkerPath, done, entCh, dirCh)
		ok(b, err)

		// wait for all workers to terminate
		wg.Wait()

		// wait for final result
		<-resCh

		b.Logf("max duration for a dir: %v", max)
	}
}
