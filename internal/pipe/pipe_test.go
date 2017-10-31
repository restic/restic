package pipe_test

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/pipe"
	rtest "github.com/restic/restic/internal/test"
)

type stats struct {
	dirs, files int
}

func acceptAll(string, os.FileInfo) bool {
	return true
}

func statPath(path string) (stats, error) {
	var s stats

	// count files and directories with filepath.Walk()
	err := filepath.Walk(rtest.TestWalkerPath, func(p string, fi os.FileInfo, err error) error {
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

const maxWorkers = 100

func TestPipelineWalkerWithSplit(t *testing.T) {
	if rtest.TestWalkerPath == "" {
		t.Skipf("walkerpath not set, skipping TestPipelineWalker")
	}

	var err error
	if !filepath.IsAbs(rtest.TestWalkerPath) {
		rtest.TestWalkerPath, err = filepath.Abs(rtest.TestWalkerPath)
		rtest.OK(t, err)
	}

	before, err := statPath(rtest.TestWalkerPath)
	rtest.OK(t, err)

	t.Logf("walking path %s with %d dirs, %d files", rtest.TestWalkerPath,
		before.dirs, before.files)

	// account for top level dir
	before.dirs++

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

				e.Result() <- true

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

				dir.Result() <- true
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

	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go worker(&wg, done, entCh, dirCh)
	}

	jobs := make(chan pipe.Job, 200)
	wg.Add(1)
	go func() {
		pipe.Split(jobs, dirCh, entCh)
		close(entCh)
		close(dirCh)
		wg.Done()
	}()

	resCh := make(chan pipe.Result, 1)
	pipe.Walk(context.TODO(), []string{rtest.TestWalkerPath}, acceptAll, jobs, resCh)

	// wait for all workers to terminate
	wg.Wait()

	// wait for top-level blob
	<-resCh

	t.Logf("walked path %s with %d dirs, %d files", rtest.TestWalkerPath,
		after.dirs, after.files)

	rtest.Assert(t, before == after, "stats do not match, expected %v, got %v", before, after)
}

func TestPipelineWalker(t *testing.T) {
	if rtest.TestWalkerPath == "" {
		t.Skipf("walkerpath not set, skipping TestPipelineWalker")
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	var err error
	if !filepath.IsAbs(rtest.TestWalkerPath) {
		rtest.TestWalkerPath, err = filepath.Abs(rtest.TestWalkerPath)
		rtest.OK(t, err)
	}

	before, err := statPath(rtest.TestWalkerPath)
	rtest.OK(t, err)

	t.Logf("walking path %s with %d dirs, %d files", rtest.TestWalkerPath,
		before.dirs, before.files)

	// account for top level dir
	before.dirs++

	after := stats{}
	m := sync.Mutex{}

	worker := func(ctx context.Context, wg *sync.WaitGroup, jobs <-chan pipe.Job) {
		defer wg.Done()
		for {
			select {
			case job, ok := <-jobs:
				if !ok {
					// channel is closed
					return
				}
				rtest.Assert(t, job != nil, "job is nil")

				switch j := job.(type) {
				case pipe.Dir:
					// wait for all content
					for _, ch := range j.Entries {
						<-ch
					}

					m.Lock()
					after.dirs++
					m.Unlock()

					j.Result() <- true
				case pipe.Entry:
					m.Lock()
					after.files++
					m.Unlock()

					j.Result() <- true
				}

			case <-ctx.Done():
				// pipeline was cancelled
				return
			}
		}
	}

	var wg sync.WaitGroup
	jobs := make(chan pipe.Job)

	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go worker(ctx, &wg, jobs)
	}

	resCh := make(chan pipe.Result, 1)
	pipe.Walk(ctx, []string{rtest.TestWalkerPath}, acceptAll, jobs, resCh)

	// wait for all workers to terminate
	wg.Wait()

	// wait for top-level blob
	<-resCh

	t.Logf("walked path %s with %d dirs, %d files", rtest.TestWalkerPath,
		after.dirs, after.files)

	rtest.Assert(t, before == after, "stats do not match, expected %v, got %v", before, after)
}

func createFile(filename, data string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	defer f.Close()

	_, err = f.Write([]byte(data))
	if err != nil {
		return err
	}

	return nil
}

func TestPipeWalkerError(t *testing.T) {
	dir, err := ioutil.TempDir("", "restic-test-")
	rtest.OK(t, err)

	base := filepath.Base(dir)

	var testjobs = []struct {
		path []string
		err  bool
	}{
		{[]string{base, "a", "file_a"}, false},
		{[]string{base, "a"}, false},
		{[]string{base, "b"}, true},
		{[]string{base, "c", "file_c"}, false},
		{[]string{base, "c"}, false},
		{[]string{base}, false},
		{[]string{}, false},
	}

	rtest.OK(t, os.Mkdir(filepath.Join(dir, "a"), 0755))
	rtest.OK(t, os.Mkdir(filepath.Join(dir, "b"), 0755))
	rtest.OK(t, os.Mkdir(filepath.Join(dir, "c"), 0755))

	rtest.OK(t, createFile(filepath.Join(dir, "a", "file_a"), "file a"))
	rtest.OK(t, createFile(filepath.Join(dir, "b", "file_b"), "file b"))
	rtest.OK(t, createFile(filepath.Join(dir, "c", "file_c"), "file c"))

	ranHook := false
	testdir := filepath.Join(dir, "b")

	// install hook that removes the dir right before readdirnames()
	debug.Hook("pipe.readdirnames", func(context interface{}) {
		path := context.(string)

		if path != testdir {
			return
		}

		t.Logf("in hook, removing test file %v", testdir)
		ranHook = true

		rtest.OK(t, os.RemoveAll(testdir))
	})

	ctx, cancel := context.WithCancel(context.TODO())

	ch := make(chan pipe.Job)
	resCh := make(chan pipe.Result, 1)

	go pipe.Walk(ctx, []string{dir}, acceptAll, ch, resCh)

	i := 0
	for job := range ch {
		if i == len(testjobs) {
			t.Errorf("too many jobs received")
			break
		}

		p := filepath.Join(testjobs[i].path...)
		if p != job.Path() {
			t.Errorf("job %d has wrong path: expected %q, got %q", i, p, job.Path())
		}

		if testjobs[i].err {
			if job.Error() == nil {
				t.Errorf("job %d expected error but got nil", i)
			}
		} else {
			if job.Error() != nil {
				t.Errorf("job %d expected no error but got %v", i, job.Error())
			}
		}

		i++
	}

	if i != len(testjobs) {
		t.Errorf("expected %d jobs, got %d", len(testjobs), i)
	}

	cancel()

	rtest.Assert(t, ranHook, "hook did not run")
	rtest.OK(t, os.RemoveAll(dir))
}

func BenchmarkPipelineWalker(b *testing.B) {
	if rtest.TestWalkerPath == "" {
		b.Skipf("walkerpath not set, skipping BenchPipelineWalker")
	}

	var max time.Duration
	m := sync.Mutex{}

	fileWorker := func(ctx context.Context, wg *sync.WaitGroup, ch <-chan pipe.Entry) {
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

				e.Result() <- true
			case <-ctx.Done():
				// pipeline was cancelled
				return
			}
		}
	}

	dirWorker := func(ctx context.Context, wg *sync.WaitGroup, ch <-chan pipe.Dir) {
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

				dir.Result() <- true
			case <-ctx.Done():
				// pipeline was cancelled
				return
			}
		}
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	for i := 0; i < b.N; i++ {
		max = 0
		entCh := make(chan pipe.Entry, 200)
		dirCh := make(chan pipe.Dir, 200)

		var wg sync.WaitGroup
		b.Logf("starting %d workers", maxWorkers)
		for i := 0; i < maxWorkers; i++ {
			wg.Add(2)
			go dirWorker(ctx, &wg, dirCh)
			go fileWorker(ctx, &wg, entCh)
		}

		jobs := make(chan pipe.Job, 200)
		wg.Add(1)
		go func() {
			pipe.Split(jobs, dirCh, entCh)
			close(entCh)
			close(dirCh)
			wg.Done()
		}()

		resCh := make(chan pipe.Result, 1)
		pipe.Walk(ctx, []string{rtest.TestWalkerPath}, acceptAll, jobs, resCh)

		// wait for all workers to terminate
		wg.Wait()

		// wait for final result
		<-resCh

		b.Logf("max duration for a dir: %v", max)
	}
}

func TestPipelineWalkerMultiple(t *testing.T) {
	if rtest.TestWalkerPath == "" {
		t.Skipf("walkerpath not set, skipping TestPipelineWalker")
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	paths, err := filepath.Glob(filepath.Join(rtest.TestWalkerPath, "*"))
	rtest.OK(t, err)

	before, err := statPath(rtest.TestWalkerPath)
	rtest.OK(t, err)

	t.Logf("walking paths %v with %d dirs, %d files", paths,
		before.dirs, before.files)

	after := stats{}
	m := sync.Mutex{}

	worker := func(ctx context.Context, wg *sync.WaitGroup, jobs <-chan pipe.Job) {
		defer wg.Done()
		for {
			select {
			case job, ok := <-jobs:
				if !ok {
					// channel is closed
					return
				}
				rtest.Assert(t, job != nil, "job is nil")

				switch j := job.(type) {
				case pipe.Dir:
					// wait for all content
					for _, ch := range j.Entries {
						<-ch
					}

					m.Lock()
					after.dirs++
					m.Unlock()

					j.Result() <- true
				case pipe.Entry:
					m.Lock()
					after.files++
					m.Unlock()

					j.Result() <- true
				}

			case <-ctx.Done():
				// pipeline was cancelled
				return
			}
		}
	}

	var wg sync.WaitGroup
	jobs := make(chan pipe.Job)

	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go worker(ctx, &wg, jobs)
	}

	resCh := make(chan pipe.Result, 1)
	pipe.Walk(ctx, paths, acceptAll, jobs, resCh)

	// wait for all workers to terminate
	wg.Wait()

	// wait for top-level blob
	<-resCh

	t.Logf("walked %d paths with %d dirs, %d files", len(paths), after.dirs, after.files)

	rtest.Assert(t, before == after, "stats do not match, expected %v, got %v", before, after)
}

func dirsInPath(path string) int {
	if path == "/" || path == "." || path == "" {
		return 0
	}

	n := 0
	for dir := path; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		n++
	}

	return n
}

func TestPipeWalkerRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("not running TestPipeWalkerRoot on %s", runtime.GOOS)
		return
	}

	cwd, err := os.Getwd()
	rtest.OK(t, err)

	testPaths := []string{
		string(filepath.Separator),
		".",
		cwd,
	}

	for _, path := range testPaths {
		testPipeWalkerRootWithPath(path, t)
	}
}

func testPipeWalkerRootWithPath(path string, t *testing.T) {
	pattern := filepath.Join(path, "*")
	rootPaths, err := filepath.Glob(pattern)
	rtest.OK(t, err)

	for i, p := range rootPaths {
		rootPaths[i], err = filepath.Rel(path, p)
		rtest.OK(t, err)
	}

	t.Logf("paths in %v (pattern %q) expanded to %v items", path, pattern, len(rootPaths))

	jobCh := make(chan pipe.Job)
	var jobs []pipe.Job

	worker := func(wg *sync.WaitGroup) {
		defer wg.Done()
		for job := range jobCh {
			jobs = append(jobs, job)
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go worker(&wg)

	filter := func(p string, fi os.FileInfo) bool {
		p, err := filepath.Rel(path, p)
		rtest.OK(t, err)
		return dirsInPath(p) <= 1
	}

	resCh := make(chan pipe.Result, 1)
	pipe.Walk(context.TODO(), []string{path}, filter, jobCh, resCh)

	wg.Wait()

	t.Logf("received %d jobs", len(jobs))

	for i, job := range jobs[:len(jobs)-1] {
		path := job.Path()
		if path == "." || path == ".." || path == string(filepath.Separator) {
			t.Errorf("job %v has invalid path %q", i, path)
		}
	}

	lastPath := jobs[len(jobs)-1].Path()
	if lastPath != "" {
		t.Errorf("last job has non-empty path %q", lastPath)
	}

	if len(jobs) < len(rootPaths) {
		t.Errorf("want at least %v jobs, got %v for path %v\n", len(rootPaths), len(jobs), path)
	}
}
