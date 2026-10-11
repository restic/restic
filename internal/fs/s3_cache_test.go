package fs

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	rtest "github.com/restic/restic/internal/test"
)

func newS3TestFSWithLimit(t testing.TB, client S3Client, limit int) *s3FS {
	filesys, err := NewS3(context.TODO(), client, "b", S3Options{CacheEntries: limit})
	rtest.OK(t, err)
	return filesys.(*s3FS)
}

func checkS3TestTree(t testing.TB, filesys FS) {
	tree := make(map[string][]byte)
	walkS3(t, filesys, "/b", tree)
	if !cmp.Equal(s3TestTree, tree, cmp.Comparer(func(a, b []byte) bool { return string(a) == string(b) && (a == nil) == (b == nil) })) {
		t.Error(cmp.Diff(s3TestTree, tree))
	}
}

func TestS3CacheLargeDirectory(t *testing.T) {
	objects := make(map[string][]byte)
	for i := range 1000 {
		objects[fmt.Sprintf("obj%04d", i)] = []byte{byte(i)}
	}
	client := NewS3MemClient(objects, s3TestModTime)
	filesys := newS3TestFSWithLimit(t, client, 100)

	// the directory is kept although it exceeds the limit
	tree := make(map[string][]byte)
	walkS3(t, filesys, "/b", tree)
	rtest.Equals(t, 1001, len(tree))
	rtest.Equals(t, 1, client.Lists)
	rtest.Equals(t, 0, client.Stats)
}

func TestS3CacheRelist(t *testing.T) {
	objects := map[string][]byte{"c/x": []byte("c")}
	for i := range 50 {
		objects[fmt.Sprintf("a/x%02d", i)] = []byte("a")
		objects[fmt.Sprintf("b/x%02d", i)] = []byte("b")
	}
	client := NewS3MemClient(objects, s3TestModTime)
	filesys := newS3TestFSWithLimit(t, client, 60)

	for _, dir := range []string{"/b", "/b/a", "/b/b"} {
		_, err := Readdirnames(filesys, dir, 0)
		rtest.OK(t, err)
	}
	rtest.Equals(t, 3, client.Lists)

	// listing b/ evicted a/, which is listed again as a whole on first use
	client.Lists = 0
	for i := range 50 {
		_, err := filesys.Lstat(fmt.Sprintf("/b/a/x%02d", i))
		rtest.OK(t, err)
	}
	rtest.Equals(t, 1, client.Lists)
	rtest.Equals(t, 0, client.Stats)

	// a directory which was never listed is not listed for a single lookup
	client.Lists = 0
	_, err := filesys.Lstat("/b/c/x")
	rtest.OK(t, err)
	rtest.Equals(t, 1, client.Stats)
	// checking whether a directory c/x/ exists
	rtest.Equals(t, 1, client.Lists)
}

func TestS3CacheKeepParents(t *testing.T) {
	objects := make(map[string][]byte)
	for i := range 200 {
		objects[fmt.Sprintf("a%03d", i)] = []byte("root")
		objects[fmt.Sprintf("upload/x%03d", i)] = []byte("upload")
		objects[fmt.Sprintf("v/x%03d", i)] = []byte("v")
	}
	client := NewS3MemClient(objects, s3TestModTime)
	filesys := newS3TestFSWithLimit(t, client, 100)

	lstat := func(format string) {
		for i := range 200 {
			_, err := filesys.Lstat(fmt.Sprintf(format, i))
			rtest.OK(t, err)
		}
	}

	// the order in which the archiver walks the tree
	_, err := Readdirnames(filesys, "/b", 0)
	rtest.OK(t, err)
	lstat("/b/a%03d")
	_, err = Readdirnames(filesys, "/b/upload", 0)
	rtest.OK(t, err)
	lstat("/b/upload/x%03d")
	_, err = Readdirnames(filesys, "/b/v", 0)
	rtest.OK(t, err)
	lstat("/b/v/x%03d")

	// the parent was kept while working on its subdirectories
	rtest.Equals(t, 3, client.Lists)
	rtest.Equals(t, 0, client.Stats)

	// the finished directory was evicted
	_, ok := filesys.cache.dirs["/b/upload"]
	rtest.Assert(t, !ok, "finished directory was not evicted")
}

func TestS3CacheCollision(t *testing.T) {
	client := NewS3MemClient(s3TestObjects, s3TestModTime)
	filesys := newS3TestFSWithLimit(t, client, 0)
	// all names collide
	filesys.cache.hash = func(string) uint64 { return 0 }

	checkS3TestTree(t, filesys)
	rtest.Assert(t, client.Stats > 0, "expected HEAD requests for colliding names")
}

func TestS3CacheConcurrent(t *testing.T) {
	client := NewS3MemClient(s3TestObjects, s3TestModTime)
	// a tiny limit causes evictions and listings to be repeated
	filesys := newS3TestFSWithLimit(t, client, 3)

	t.Run("group", func(t *testing.T) {
		for i := range 8 {
			t.Run(fmt.Sprint(i), func(t *testing.T) {
				t.Parallel()
				checkS3TestTree(t, filesys)
			})
		}
	})
}
