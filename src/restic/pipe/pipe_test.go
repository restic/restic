package pipe_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"restic/debug"
	"restic/pipe"
	. "restic/test"
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
	err := filepath.Walk(TestWalkerPath, func(p string, fi os.FileInfo, err error) error {
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
	if TestWalkerPath == "" {
		t.Skipf("walkerpath not set, skipping TestPipelineWalker")
	}

	var err error
	if !filepath.IsAbs(TestWalkerPath) {
		TestWalkerPath, err = filepath.Abs(TestWalkerPath)
		OK(t, err)
	}

	before, err := statPath(TestWalkerPath)
	OK(t, err)

	t.Logf("walking path %s with %d dirs, %d files", TestWalkerPath,
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
	pipe.Walk([]string{TestWalkerPath}, acceptAll, done, jobs, resCh)

	// wait for all workers to terminate
	wg.Wait()

	// wait for top-level blob
	<-resCh

	t.Logf("walked path %s with %d dirs, %d files", TestWalkerPath,
		after.dirs, after.files)

	Assert(t, before == after, "stats do not match, expected %v, got %v", before, after)
}

func TestPipelineWalker(t *testing.T) {
	if TestWalkerPath == "" {
		t.Skipf("walkerpath not set, skipping TestPipelineWalker")
	}

	var err error
	if !filepath.IsAbs(TestWalkerPath) {
		TestWalkerPath, err = filepath.Abs(TestWalkerPath)
		OK(t, err)
	}

	before, err := statPath(TestWalkerPath)
	OK(t, err)

	t.Logf("walking path %s with %d dirs, %d files", TestWalkerPath,
		before.dirs, before.files)

	// account for top level dir
	before.dirs++

	after := stats{}
	m := sync.Mutex{}

	worker := func(wg *sync.WaitGroup, done <-chan struct{}, jobs <-chan pipe.Job) {
		defer wg.Done()
		for {
			select {
			case job, ok := <-jobs:
				if !ok {
					// channel is closed
					return
				}
				Assert(t, job != nil, "job is nil")

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

			case <-done:
				// pipeline was cancelled
				return
			}
		}
	}

	var wg sync.WaitGroup
	done := make(chan struct{})
	jobs := make(chan pipe.Job)

	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go worker(&wg, done, jobs)
	}

	resCh := make(chan pipe.Result, 1)
	pipe.Walk([]string{TestWalkerPath}, acceptAll, done, jobs, resCh)

	// wait for all workers to terminate
	wg.Wait()

	// wait for top-level blob
	<-resCh

	t.Logf("walked path %s with %d dirs, %d files", TestWalkerPath,
		after.dirs, after.files)

	Assert(t, before == after, "stats do not match, expected %v, got %v", before, after)
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
	OK(t, err)

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

	OK(t, os.Mkdir(filepath.Join(dir, "a"), 0755))
	OK(t, os.Mkdir(filepath.Join(dir, "b"), 0755))
	OK(t, os.Mkdir(filepath.Join(dir, "c"), 0755))

	OK(t, createFile(filepath.Join(dir, "a", "file_a"), "file a"))
	OK(t, createFile(filepath.Join(dir, "b", "file_b"), "file b"))
	OK(t, createFile(filepath.Join(dir, "c", "file_c"), "file c"))

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

		OK(t, os.RemoveAll(testdir))
	})

	done := make(chan struct{})
	ch := make(chan pipe.Job)
	resCh := make(chan pipe.Result, 1)

	go pipe.Walk([]string{dir}, acceptAll, done, ch, resCh)

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

	close(done)

	Assert(t, ranHook, "hook did not run")
	OK(t, os.RemoveAll(dir))
}

func BenchmarkPipelineWalker(b *testing.B) {
	if TestWalkerPath == "" {
		b.Skipf("walkerpath not set, skipping BenchPipelineWalker")
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

				e.Result() <- true
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

				dir.Result() <- true
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
		b.Logf("starting %d workers", maxWorkers)
		for i := 0; i < maxWorkers; i++ {
			wg.Add(2)
			go dirWorker(&wg, done, dirCh)
			go fileWorker(&wg, done, entCh)
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
		pipe.Walk([]string{TestWalkerPath}, acceptAll, done, jobs, resCh)

		// wait for all workers to terminate
		wg.Wait()

		// wait for final result
		<-resCh

		b.Logf("max duration for a dir: %v", max)
	}
}

func TestPipelineWalkerMultiple(t *testing.T) {
	if TestWalkerPath == "" {
		t.Skipf("walkerpath not set, skipping TestPipelineWalker")
	}

	paths, err := filepath.Glob(filepath.Join(TestWalkerPath, "*"))
	OK(t, err)

	before, err := statPath(TestWalkerPath)
	OK(t, err)

	t.Logf("walking paths %v with %d dirs, %d files", paths,
		before.dirs, before.files)

	after := stats{}
	m := sync.Mutex{}

	worker := func(wg *sync.WaitGroup, done <-chan struct{}, jobs <-chan pipe.Job) {
		defer wg.Done()
		for {
			select {
			case job, ok := <-jobs:
				if !ok {
					// channel is closed
					return
				}
				Assert(t, job != nil, "job is nil")

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

			case <-done:
				// pipeline was cancelled
				return
			}
		}
	}

	var wg sync.WaitGroup
	done := make(chan struct{})
	jobs := make(chan pipe.Job)

	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go worker(&wg, done, jobs)
	}

	resCh := make(chan pipe.Result, 1)
	pipe.Walk(paths, acceptAll, done, jobs, resCh)

	// wait for all workers to terminate
	wg.Wait()

	// wait for top-level blob
	<-resCh

	t.Logf("walked %d paths with %d dirs, %d files", len(paths), after.dirs, after.files)

	Assert(t, before == after, "stats do not match, expected %v, got %v", before, after)
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
	OK(t, err)

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
	OK(t, err)

	for i, p := range rootPaths {
		rootPaths[i], err = filepath.Rel(path, p)
		OK(t, err)
	}

	t.Logf("paths in %v (pattern %q) expanded to %v items", path, pattern, len(rootPaths))

	done := make(chan struct{})
	defer close(done)

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
		OK(t, err)
		return dirsInPath(p) <= 1
	}

	resCh := make(chan pipe.Result, 1)
	pipe.Walk([]string{path}, filter, done, jobCh, resCh)

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
