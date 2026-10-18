package archiver

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/restic/restic/internal/data"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

func TestArchiverS3Source(t *testing.T) {
	ctx := t.Context()
	modTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	client := fs.NewS3MemClient(map[string][]byte{
		"a":         []byte("object a"),
		"a/b":       []byte("object a/b"),
		"x//y":      []byte("object x//y"),
		"dir/":      nil,
		"dir/file":  []byte("object dir/file"),
		"empty/":    nil,
		"../escape": []byte("object ../escape"),
	}, modTime)
	want := TestDir{
		"bucket": TestDir{
			"a¤":    TestFile{Content: "object a"},
			"a":     TestDir{"b": TestFile{Content: "object a/b"}},
			"x":     TestDir{"¤": TestDir{"y": TestFile{Content: "object x//y"}}},
			"dir":   TestDir{"file": TestFile{Content: "object dir/file"}},
			"empty": TestDir{},
			"¤2E¤2E": TestDir{
				"escape": TestFile{Content: "object ../escape"},
			},
		},
	}

	filesys, err := fs.NewS3(ctx, client, "bucket", fs.S3Options{})
	rtest.OK(t, err)
	repo := repository.TestRepository(t)
	arch := New(repo, filesys, Options{})

	sn, id, summary, err := arch.Snapshot(ctx, []string{"/bucket"}, SnapshotOptions{Time: time.Now()})
	rtest.OK(t, err)
	TestEnsureSnapshot(t, repo, id, want)
	// snapshot paths are made absolute using the rules of the local OS, as
	// for --stdin
	wantPath, err := filepath.Abs("/bucket")
	rtest.OK(t, err)
	rtest.Equals(t, []string{wantPath}, sn.Paths)
	rtest.Equals(t, ChangeStats{New: 5}, summary.Files)
	rtest.Equals(t, 5, client.Gets)

	// unchanged objects must not be read again
	client.Gets = 0
	client.Stats = 0
	sn2, id2, summary, err := arch.Snapshot(ctx, []string{"/bucket"}, SnapshotOptions{Time: time.Now(), ParentSnapshot: sn})
	rtest.OK(t, err)
	TestEnsureSnapshot(t, repo, id2, want)
	rtest.Equals(t, ChangeStats{Unchanged: 5}, summary.Files)
	rtest.Equals(t, 0, client.Gets)
	rtest.Equals(t, 0, client.Stats)
	rtest.Equals(t, *sn.Tree, *sn2.Tree)

	// same size and modification time, only the ETag differs
	client.Put("a/b", []byte("OBJECT a/b"), modTime)
	filesys, err = fs.NewS3(ctx, client, "bucket", fs.S3Options{})
	rtest.OK(t, err)
	arch = New(repo, filesys, Options{})
	_, id3, summary, err := arch.Snapshot(ctx, []string{"/bucket"}, SnapshotOptions{Time: time.Now(), ParentSnapshot: sn2})
	rtest.OK(t, err)
	rtest.Equals(t, ChangeStats{Changed: 1, Unchanged: 4}, summary.Files)
	want["bucket"].(TestDir)["a"].(TestDir)["b"] = TestFile{Content: "OBJECT a/b"}
	TestEnsureSnapshot(t, repo, id3, want)

}

func TestArchiverS3SourceWideDirectories(t *testing.T) {
	ctx := t.Context()
	objects := make(map[string][]byte)
	for i := range 300 {
		objects[fmt.Sprintf("a%03d", i)] = []byte("root")
		objects[fmt.Sprintf("upload/x%03d", i)] = []byte("upload")
		objects[fmt.Sprintf("z%03d", i)] = []byte("root")
	}
	client := fs.NewS3MemClient(objects, time.Now())

	// each directory exceeds the cache limit
	filesys, err := fs.NewS3(ctx, client, "bucket", fs.S3Options{CacheEntries: 100})
	rtest.OK(t, err)
	repo := repository.TestRepository(t)
	arch := New(repo, filesys, Options{})

	_, _, summary, err := arch.Snapshot(ctx, []string{"/bucket"}, SnapshotOptions{Time: time.Now()})
	rtest.OK(t, err)
	rtest.Equals(t, ChangeStats{New: 900}, summary.Files)
	// each directory is listed once, and no object needs a HEAD request
	rtest.Equals(t, 2, client.Lists)
	rtest.Equals(t, 0, client.Stats)
}

func TestArchiverS3SourceConcurrentReads(t *testing.T) {
	ctx := t.Context()
	objects := make(map[string][]byte)
	for i := range 50 {
		objects[fmt.Sprintf("o%02d", i)] = []byte(fmt.Sprintf("small object %d", i))
	}
	client := fs.NewS3MemClient(objects, time.Now())
	var inFlight, maxInFlight atomic.Int32
	client.BeforeGet = func(string) {
		storeMax(&maxInFlight, inFlight.Add(1))
		time.Sleep(10 * time.Millisecond)
		inFlight.Add(-1)
	}

	filesys, err := fs.NewS3(ctx, client, "bucket", fs.S3Options{})
	rtest.OK(t, err)
	arch := New(repository.TestRepository(t), filesys, Options{ReadConcurrency: 4})
	_, _, _, err = arch.Snapshot(ctx, []string{"/bucket"}, SnapshotOptions{Time: time.Now()})
	rtest.OK(t, err)

	// requesting the objects while walking the tree would serialize them
	rtest.Assert(t, maxInFlight.Load() > 1, "objects were not requested concurrently")
}

// storeMax sets v to n if n is larger than its current value.
func storeMax(v *atomic.Int32, n int32) {
	for {
		m := v.Load()
		if n <= m || v.CompareAndSwap(m, n) {
			return
		}
	}
}

func TestArchiverS3SourceModified(t *testing.T) {
	ctx := t.Context()
	oldTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	newTime := oldTime.Add(time.Hour)
	client := fs.NewS3MemClient(map[string][]byte{"obj": []byte("old")}, oldTime)
	client.BeforeGet = func(key string) {
		client.BeforeGet = nil
		client.Put(key, []byte("new content"), newTime)
	}

	filesys, err := fs.NewS3(ctx, client, "bucket", fs.S3Options{})
	rtest.OK(t, err)
	repo := repository.TestRepository(t)
	arch := New(repo, filesys, Options{})
	sn, id, _, err := arch.Snapshot(ctx, []string{"/bucket"}, SnapshotOptions{Time: time.Now()})
	rtest.OK(t, err)
	TestEnsureSnapshot(t, repo, id, TestDir{"bucket": TestDir{"obj": TestFile{Content: "new content"}}})

	// the metadata describes the version which was read
	node := loadS3TestNode(ctx, t, repo, *sn.Tree, "bucket", "obj")
	rtest.Equals(t, uint64(len("new content")), node.Size)
	rtest.Equals(t, newTime, node.ModTime.UTC())
}

func loadS3TestNode(ctx context.Context, t *testing.T, repo restic.BlobLoader, id restic.ID, names ...string) *data.Node {
	for i, name := range names {
		tree, err := data.LoadTree(ctx, repo, id)
		rtest.OK(t, err)
		var found *data.Node
		for item := range tree {
			rtest.OK(t, item.Error)
			if item.Node.Name == name {
				found = item.Node
			}
		}
		rtest.Assert(t, found != nil, "node %v not found", name)
		if i == len(names)-1 {
			return found
		}
		id = *found.Subtree
	}
	return nil
}

func TestArchiverS3SourcePrefetch(t *testing.T) {
	objects := make(map[string][]byte)
	for i := range 20 {
		for j := range 3 {
			for k := range 2 {
				objects[fmt.Sprintf("d%02d/s%d/o%d", i, j, k)] = []byte("small")
			}
		}
	}
	const dirs = 1 + 20 + 20*3

	for _, test := range []struct {
		prefetch int
		exclude  string
		files    uint
	}{
		{0, "", 120},
		{8, "", 120},
		{8, "/bucket/d0", 60},
	} {
		t.Run(fmt.Sprintf("prefetch=%d,exclude=%q", test.prefetch, test.exclude), func(t *testing.T) {
			ctx := t.Context()
			client := fs.NewS3MemClient(objects, time.Now())
			var inFlight, maxInFlight atomic.Int32
			client.BeforeList = func(string) {
				storeMax(&maxInFlight, inFlight.Add(1))
				time.Sleep(10 * time.Millisecond)
				inFlight.Add(-1)
			}

			filesys, err := fs.NewS3(ctx, client, "bucket", fs.S3Options{PrefetchDirs: test.prefetch})
			rtest.OK(t, err)
			arch := New(repository.TestRepository(t), filesys, Options{})
			if test.exclude != "" {
				arch.SelectByName = func(item string) bool { return !strings.HasPrefix(item, test.exclude) }
			}
			start := time.Now()
			_, _, summary, err := arch.Snapshot(ctx, []string{"/bucket"}, SnapshotOptions{Time: time.Now()})
			rtest.OK(t, err)
			t.Logf("%v, %d listings, max %d concurrent", time.Since(start).Round(time.Millisecond), client.Lists, maxInFlight.Load())

			rtest.Equals(t, test.files, summary.Files.New)
			if test.prefetch == 0 {
				rtest.Equals(t, dirs, client.Lists)
				rtest.Equals(t, int32(1), maxInFlight.Load())
				return
			}
			rtest.Assert(t, maxInFlight.Load() > 1, "directories were not listed concurrently")
			if test.exclude == "" {
				// no directory is listed twice
				rtest.Equals(t, dirs, client.Lists)
			}
		})
	}
}
