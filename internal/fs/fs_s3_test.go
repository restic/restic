package fs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	rtest "github.com/restic/restic/internal/test"
)

// TestS3NameEncodingUnique checks that all short names over an alphabet
// containing the escape sequences are encoded to distinct names.
func TestS3NameEncodingUnique(t *testing.T) {
	alphabet := []string{"¤", "A", "4", "2", "E", ".", "x"}
	seen := make(map[string]string)
	var gen func(prefix string, n int)
	gen = func(prefix string, n int) {
		for _, shadowed := range []bool{false, true} {
			name := s3EncodeObjectName(prefix, shadowed)
			id := fmt.Sprintf("%q/%v", prefix, shadowed)
			if other, ok := seen[name]; ok {
				t.Fatalf("%v and %v both encode to %q", id, other, name)
			}
			seen[name] = id
			raw, sh, err := s3DecodeName(name)
			rtest.OK(t, err)
			rtest.Equals(t, prefix, raw)
			rtest.Equals(t, shadowed, sh)
		}
		if n == 0 {
			return
		}
		for _, c := range alphabet {
			gen(prefix+c, n-1)
		}
	}
	gen("", 5)
}

func TestS3NameEncoding(t *testing.T) {
	for _, test := range []struct {
		raw      string
		shadowed bool
		name     string
	}{
		{"foo", false, "foo"},
		{"foo", true, "foo¤"},
		{"", false, "¤"},
		{"", true, "¤¤"},
		{".", false, "¤2E"},
		{"..", true, "¤2E¤2E¤"},
		{"...", false, "..."},
		{"¤", false, "¤A4"},
		{"¤", true, "¤A4¤"},
		{"¤2E", false, "¤A42E"},
		{"10¤¤", false, "10¤A4¤A4"},
		{"a\nb\x7f\\", false, "a\nb\x7f\\"},
		{"blåbær", false, "blåbær"},
	} {
		name := s3EncodeObjectName(test.raw, test.shadowed)
		rtest.Equals(t, test.name, name)
		raw, shadowed, err := s3DecodeName(name)
		rtest.OK(t, err)
		rtest.Equals(t, test.raw, raw)
		rtest.Equals(t, test.shadowed, shadowed)
	}

	for _, name := range []string{"¤41", "¤a4", "¤2e", "a¤2E", "¤¤¤", "x¤¤", "¤A", "¤x", ".", ".."} {
		_, _, err := s3DecodeName(name)
		rtest.Assert(t, err != nil, "expected error for %q", name)
	}
}

var s3TestModTime = time.Date(2026, 1, 2, 3, 4, 5, 678e6, time.UTC)

// s3TestObjects contains keys which cannot be represented in a file system
// as is. s3TestTree is the expected tree, nil values are directories.
var s3TestObjects = map[string][]byte{
	"plain.txt":  []byte("plain"),
	"dir/":       nil,
	"dir/file":   []byte("file in dir"),
	"a":          []byte("shadowed by directory a/"),
	"a/b":        []byte("a/b"),
	"x//y":       []byte("double slash"),
	"x///y":      []byte("triple slash"),
	"e/":         []byte("marker with content"),
	"e//f":       []byte("e//f"),
	"price¤.txt": []byte("currency sign"),
	"./dot":      []byte("dot"),
	"../up":      []byte("dotdot"),
	"empty/":     nil,
	"ctl\nname":  []byte("newline"),
	"/lead":      []byte("leading slash"),
}

var s3TestTree = map[string][]byte{
	"/b":              nil,
	"/b/plain.txt":    []byte("plain"),
	"/b/dir":          nil,
	"/b/dir/file":     []byte("file in dir"),
	"/b/a¤":           []byte("shadowed by directory a/"),
	"/b/a":            nil,
	"/b/a/b":          []byte("a/b"),
	"/b/x":            nil,
	"/b/x/¤":          nil,
	"/b/x/¤/y":        []byte("double slash"),
	"/b/x/¤/¤":        nil,
	"/b/x/¤/¤/y":      []byte("triple slash"),
	"/b/e":            nil,
	"/b/e/¤¤":         []byte("marker with content"),
	"/b/e/¤":          nil,
	"/b/e/¤/f":        []byte("e//f"),
	"/b/price¤A4.txt": []byte("currency sign"),
	"/b/¤2E":          nil,
	"/b/¤2E/dot":      []byte("dot"),
	"/b/¤2E¤2E":       nil,
	"/b/¤2E¤2E/up":    []byte("dotdot"),
	"/b/empty":        nil,
	"/b/ctl\nname":    []byte("newline"),
	"/b/¤":            nil,
	"/b/¤/lead":       []byte("leading slash"),
}

func newS3TestFS(t testing.TB, client S3Client) FS {
	filesys, err := NewS3(context.TODO(), client, "b", S3Options{})
	rtest.OK(t, err)
	return filesys
}

func walkS3(t testing.TB, filesys FS, p string, result map[string][]byte) {
	f, err := filesys.OpenFile(p, O_NOFOLLOW, true)
	rtest.OK(t, err)
	defer func() { rtest.OK(t, f.Close()) }()

	fi, err := f.Stat()
	rtest.OK(t, err)
	rtest.Equals(t, path.Base(p), fi.Name)
	if !fi.Mode.IsDir() {
		rtest.OK(t, f.MakeReadable())
		buf, err := io.ReadAll(f)
		rtest.OK(t, err)
		rtest.Equals(t, fi.Size, int64(len(buf)))
		result[p] = buf
		return
	}

	result[p] = nil
	names, err := f.Readdirnames(-1)
	rtest.OK(t, err)
	for _, name := range names {
		walkS3(t, filesys, path.Join(p, name), result)
	}
}

func TestS3Tree(t *testing.T) {
	client := NewS3MemClient(s3TestObjects, s3TestModTime)
	filesys := newS3TestFS(t, client)

	tree := make(map[string][]byte)
	walkS3(t, filesys, "/b", tree)
	if !cmp.Equal(s3TestTree, tree, cmp.Comparer(func(a, b []byte) bool { return string(a) == string(b) && (a == nil) == (b == nil) })) {
		t.Error(cmp.Diff(s3TestTree, tree))
	}
	// all metadata must come from listings
	rtest.Equals(t, 0, client.Stats)

	names, err := Readdirnames(filesys, "/", 0)
	rtest.OK(t, err)
	rtest.Equals(t, []string{"b"}, names)

	for p := range s3TestTree {
		f, err := filesys.OpenFile(p, O_NOFOLLOW, true)
		rtest.OK(t, err)
		node, err := f.ToNode(false, t.Errorf)
		rtest.OK(t, err)
		rtest.OK(t, f.Close())

		if node.Type == "file" {
			rtest.Equals(t, s3TestModTime.Truncate(time.Second), node.ModTime)
		}
	}
}

func TestS3LookupWithoutListing(t *testing.T) {
	client := NewS3MemClient(s3TestObjects, s3TestModTime)
	warm := newS3TestFS(t, client)
	walkS3(t, warm, "/b", make(map[string][]byte))

	for p := range s3TestTree {
		cold := newS3TestFS(t, client)
		want, err := warm.Lstat(p)
		rtest.OK(t, err)
		got, err := cold.Lstat(p)
		rtest.OK(t, err)
		rtest.Equals(t, want, got, "Lstat(%v)", p)
	}

	cold := newS3TestFS(t, client)
	for _, p := range []string{"/c", "/b/nope", "/b/¤41", "/b/¤A", "/b/a¤/b", "/b/plain.txt¤", "/b/dir/file/x"} {
		_, err := cold.Lstat(p)
		rtest.Assert(t, errors.Is(err, os.ErrNotExist), "Lstat(%v) returned %v", p, err)
	}
}

func TestS3ModifiedDuringBackup(t *testing.T) {
	client := NewS3MemClient(s3TestObjects, s3TestModTime)
	filesys := newS3TestFS(t, client)

	_, err := Readdirnames(filesys, "/b", 0)
	rtest.OK(t, err)
	f, err := filesys.OpenFile("/b/plain.txt", O_NOFOLLOW, true)
	rtest.OK(t, err)
	newTime := s3TestModTime.Add(time.Hour)
	client.Put("plain.txt", []byte("modified"), newTime)

	// the modified object is read, and the metadata describes it
	rtest.OK(t, f.MakeReadable())
	buf, err := io.ReadAll(f)
	rtest.OK(t, err)
	rtest.Equals(t, "modified", string(buf))
	fi, err := f.Stat()
	rtest.OK(t, err)
	rtest.Equals(t, int64(len("modified")), fi.Size)
	rtest.Equals(t, newTime.Truncate(time.Second), fi.ModTime)
	node, err := f.ToNode(false, t.Errorf)
	rtest.OK(t, err)
	rtest.Equals(t, uint64(len("modified")), node.Size)
	rtest.OK(t, f.Close())
	rtest.Equals(t, 1, client.Gets)
	rtest.Equals(t, 0, client.Stats)

	// the cache contains the new metadata
	fi2, err := filesys.Lstat("/b/plain.txt")
	rtest.OK(t, err)
	rtest.Equals(t, fi, fi2)
}

func TestS3ModifiedContinuously(t *testing.T) {
	client := NewS3MemClient(s3TestObjects, s3TestModTime)
	filesys := newS3TestFS(t, client)

	f, err := filesys.OpenFile("/b/plain.txt", O_NOFOLLOW, true)
	rtest.OK(t, err)
	n := 0
	client.BeforeGet = func(key string) {
		n++
		client.Put(key, []byte(fmt.Sprintf("version %d", n)), s3TestModTime)
	}
	// whichever version is returned is backed up with matching metadata
	rtest.OK(t, f.MakeReadable())
	buf, err := io.ReadAll(f)
	rtest.OK(t, err)
	rtest.Equals(t, "version 1", string(buf))
	fi, err := f.Stat()
	rtest.OK(t, err)
	rtest.Equals(t, int64(len(buf)), fi.Size)
	rtest.OK(t, f.Close())
	rtest.Equals(t, 1, client.Gets)
}

func TestS3DeletedDuringBackup(t *testing.T) {
	client := NewS3MemClient(map[string][]byte{"gone": []byte("gone")}, s3TestModTime)
	filesys := newS3TestFS(t, client)

	_, err := Readdirnames(filesys, "/b", 0)
	rtest.OK(t, err)
	f, err := filesys.OpenFile("/b/gone", O_NOFOLLOW, true)
	rtest.OK(t, err)
	client.BeforeGet = func(key string) { client.Delete(key) }
	rtest.OK(t, f.MakeReadable())
	_, err = f.ToNode(false, t.Errorf)
	rtest.Assert(t, errors.Is(err, os.ErrNotExist), "unexpected error %v", err)
}
