package fs

import (
	"testing"
	"time"

	rtest "github.com/restic/restic/internal/test"
)

func TestGlobS3(t *testing.T) {
	client := NewS3MemClient(map[string][]byte{
		"photos/2024-01/a.jpg": []byte("a"),
		"photos/2024-01/b.txt": []byte("b"),
		"photos/2024-02/c.jpg": []byte("c"),
		"photos/2023-12/d.jpg": []byte("d"),
		"docs/x.pdf":           []byte("x"),
	}, time.Now())

	for _, test := range []struct {
		pattern string
		want    []string
	}{
		{"/b/photos/2024-*", []string{"/b/photos/2024-01", "/b/photos/2024-02"}},
		{"/b/photos/*/*.jpg", []string{"/b/photos/2023-12/d.jpg", "/b/photos/2024-01/a.jpg", "/b/photos/2024-02/c.jpg"}},
		{"/b/photos/2024-0[12]/*.jpg", []string{"/b/photos/2024-01/a.jpg", "/b/photos/2024-02/c.jpg"}},
		{"/b/*/2024-01", []string{"/b/photos/2024-01"}},
		{"/b/docs/x.pdf", []string{"/b/docs/x.pdf"}},
		{"/b/docs//x.pdf", []string{"/b/docs/x.pdf"}},
		{"/b/nope", nil},
		{"/b/nope/*", nil},
		{"/b/docs/x.pdf/*", nil},
		{"/b/photos/2024-01/\\*", nil},
	} {
		filesys := newS3TestFS(t, client)
		matches, err := Glob(filesys, test.pattern)
		rtest.OK(t, err)
		rtest.Equals(t, test.want, matches, "pattern %q", test.pattern)
	}

	// only the directory containing the wildcard is listed, besides checking
	// that it exists
	client.Lists = 0
	_, err := Glob(newS3TestFS(t, client), "/b/photos/2024-01/*")
	rtest.OK(t, err)
	rtest.Equals(t, 2, client.Lists)

	for _, pattern := range []string{"/b/[", "b/*"} {
		_, err := Glob(newS3TestFS(t, client), pattern)
		rtest.Assert(t, err != nil, "expected error for %q", pattern)
	}
}
