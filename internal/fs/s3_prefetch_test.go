package fs

import (
	"context"
	"fmt"
	"slices"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func s3PrefetchState(p *s3Prefetcher) (windows, results []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for d := range p.windows {
		windows = append(windows, d)
	}
	for d := range p.results {
		results = append(results, d)
	}
	slices.Sort(windows)
	slices.Sort(results)
	return windows, results
}

func TestS3PrefetchWindow(t *testing.T) {
	objects := map[string][]byte{"z/x/y": []byte("z")}
	for i := range 10 {
		objects[fmt.Sprintf("d%d/x", i)] = []byte("d")
	}
	client := NewS3MemClient(objects, s3TestModTime)
	filesys, err := NewS3(context.TODO(), client, "b", S3Options{PrefetchDirs: 3})
	rtest.OK(t, err)
	p := filesys.(*s3FS).prefetch

	readdir := func(dir string) {
		t.Helper()
		_, err := Readdirnames(filesys, dir, 0)
		rtest.OK(t, err)
	}

	// the first subdirectories are listed ahead
	readdir("/b")
	windows, results := s3PrefetchState(p)
	rtest.Equals(t, []string{"/b"}, windows)
	rtest.Equals(t, []string{"/b/d0", "/b/d1", "/b/d2"}, results)

	// taking a listing moves the window
	readdir("/b/d0")
	_, results = s3PrefetchState(p)
	rtest.Equals(t, []string{"/b/d1", "/b/d2", "/b/d3"}, results)

	// skipped directories are dropped
	readdir("/b/d3")
	_, results = s3PrefetchState(p)
	rtest.Equals(t, []string{"/b/d4", "/b/d5", "/b/d6"}, results)

	// the state of the parents is kept while working on a subdirectory
	readdir("/b/z")
	windows, results = s3PrefetchState(p)
	rtest.Equals(t, []string{"/b", "/b/z"}, windows)
	rtest.Equals(t, []string{"/b/z/x"}, results)
	readdir("/b/z/x")
	windows, results = s3PrefetchState(p)
	rtest.Equals(t, []string{"/b", "/b/z"}, windows)
	rtest.Equals(t, []string(nil), results)

	// the state of finished directories is dropped
	readdir("/b/d9")
	windows, _ = s3PrefetchState(p)
	rtest.Equals(t, []string{"/b"}, windows)
}

func TestS3PrefetchDisabled(t *testing.T) {
	client := NewS3MemClient(s3TestObjects, s3TestModTime)
	filesys := newS3TestFS(t, client)
	rtest.Assert(t, filesys.(*s3FS).prefetch == nil, "prefetcher should be disabled by default")
	checkS3TestTree(t, filesys)
}

func TestS3PrefetchTree(t *testing.T) {
	client := NewS3MemClient(s3TestObjects, s3TestModTime)
	filesys, err := NewS3(context.TODO(), client, "b", S3Options{PrefetchDirs: 2})
	rtest.OK(t, err)
	checkS3TestTree(t, filesys)
}
