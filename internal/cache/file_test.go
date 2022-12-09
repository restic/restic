package cache

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"

	"golang.org/x/sync/errgroup"
)

func generateRandomFiles(t testing.TB, tpe restic.FileType, c *Cache) restic.IDSet {
	ids := restic.NewIDSet()
	for i := 0; i < rand.Intn(15)+10; i++ {
		buf := test.Random(rand.Int(), 1<<19)
		id := restic.Hash(buf)
		h := restic.Handle{Type: tpe, Name: id.String()}

		if c.Has(h) {
			t.Errorf("index %v present before save", id)
		}

		err := c.Save(h, bytes.NewReader(buf))
		if err != nil {
			t.Fatal(err)
		}
		ids.Insert(id)
	}
	return ids
}

// randomID returns a random ID from s.
func randomID(s restic.IDSet) restic.ID {
	for id := range s {
		return id
	}
	panic("set is empty")
}

func load(t testing.TB, c *Cache, h restic.Handle) []byte {
	rd, err := c.load(h, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	if rd == nil {
		t.Fatalf("load() returned nil reader")
	}

	buf, err := io.ReadAll(rd)
	if err != nil {
		t.Fatal(err)
	}

	if err = rd.Close(); err != nil {
		t.Fatal(err)
	}

	return buf
}

func listFiles(t testing.TB, c *Cache, tpe restic.FileType) restic.IDSet {
	list, err := c.list(tpe)
	if err != nil {
		t.Errorf("listing failed: %v", err)
	}

	return list
}

func clearFiles(t testing.TB, c *Cache, tpe restic.FileType, valid restic.IDSet) {
	if err := c.Clear(tpe, valid); err != nil {
		t.Error(err)
	}
}

func TestFiles(t *testing.T) {
	seed := time.Now().Unix()
	t.Logf("seed is %v", seed)
	rand.Seed(seed)

	c := TestNewCache(t)

	var tests = []restic.FileType{
		restic.SnapshotFile,
		restic.PackFile,
		restic.IndexFile,
	}

	for _, tpe := range tests {
		t.Run(tpe.String(), func(t *testing.T) {
			ids := generateRandomFiles(t, tpe, c)
			id := randomID(ids)

			h := restic.Handle{Type: tpe, Name: id.String()}
			id2 := restic.Hash(load(t, c, h))

			if !id.Equal(id2) {
				t.Errorf("wrong data returned, want %v, got %v", id.Str(), id2.Str())
			}

			if !c.Has(h) {
				t.Errorf("cache thinks index %v isn't present", id.Str())
			}

			list := listFiles(t, c, tpe)
			if !ids.Equals(list) {
				t.Errorf("wrong list of index IDs returned, want:\n  %v\ngot:\n  %v", ids, list)
			}

			clearFiles(t, c, tpe, restic.NewIDSet(id))
			list2 := listFiles(t, c, tpe)
			ids.Delete(id)
			want := restic.NewIDSet(id)
			if !list2.Equals(want) {
				t.Errorf("ClearIndexes removed indexes, want:\n  %v\ngot:\n  %v", list2, want)
			}

			clearFiles(t, c, tpe, restic.NewIDSet())
			want = restic.NewIDSet()
			list3 := listFiles(t, c, tpe)
			if !list3.Equals(want) {
				t.Errorf("ClearIndexes returned a wrong list, want:\n  %v\ngot:\n  %v", want, list3)
			}
		})
	}
}

func TestFileLoad(t *testing.T) {
	seed := time.Now().Unix()
	t.Logf("seed is %v", seed)
	rand.Seed(seed)

	c := TestNewCache(t)

	// save about 5 MiB of data in the cache
	data := test.Random(rand.Int(), 5234142)
	id := restic.ID{}
	copy(id[:], data)
	h := restic.Handle{
		Type: restic.PackFile,
		Name: id.String(),
	}
	if err := c.Save(h, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	var tests = []struct {
		offset int64
		length int
	}{
		{0, 0},
		{5, 0},
		{32*1024 + 5, 0},
		{0, 123},
		{0, 64*1024 + 234},
		{100, 5234142 - 100},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%v/%v", test.length, test.offset), func(t *testing.T) {
			rd, err := c.load(h, test.length, test.offset)
			if err != nil {
				t.Fatal(err)
			}

			buf, err := io.ReadAll(rd)
			if err != nil {
				t.Fatal(err)
			}

			if err = rd.Close(); err != nil {
				t.Fatal(err)
			}

			o := int(test.offset)
			l := test.length
			if test.length == 0 {
				l = len(data) - o
			}

			if l > len(data)-o {
				l = len(data) - o
			}

			if len(buf) != l {
				t.Fatalf("wrong number of bytes returned: want %d, got %d", l, len(buf))
			}

			if !bytes.Equal(buf, data[o:o+l]) {
				t.Fatalf("wrong data returned, want:\n  %02x\ngot:\n  %02x", data[o:o+16], buf[:16])
			}
		})
	}
}

// Simulate multiple processes writing to a cache, using goroutines.
//
// The possibility of sharing a cache between multiple concurrent restic
// processes isn't guaranteed in the docs and doesn't always work on Windows, hence the
// check on GOOS. Cache sharing is considered a "nice to have" on POSIX, for now.
//
// The cache first creates a temporary file and then renames it to its final name.
// On Windows renaming internally creates a file handle with a shareMode which
// includes FILE_SHARE_DELETE. The Go runtime opens files without FILE_SHARE_DELETE,
// thus Open(fn) will fail until the file handle used for renaming was closed.
// See https://devblogs.microsoft.com/oldnewthing/20211022-00/?p=105822
// for hints on how to fix this properly.
func TestFileSaveConcurrent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("may not work due to FILE_SHARE_DELETE issue")
	}

	const nproc = 40

	var (
		c    = TestNewCache(t)
		data = test.Random(1, 10000)
		g    errgroup.Group
		id   restic.ID
	)
	rand.Read(id[:])

	h := restic.Handle{
		Type: restic.PackFile,
		Name: id.String(),
	}

	for i := 0; i < nproc/2; i++ {
		g.Go(func() error { return c.Save(h, bytes.NewReader(data)) })

		// Can't use load because only the main goroutine may call t.Fatal.
		g.Go(func() error {
			// The timing is hard to get right, but the main thing we want to
			// ensure is ENOENT or nil error.
			time.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond)

			f, err := c.load(h, 0, 0)
			t.Logf("Load error: %v", err)
			switch {
			case err == nil:
			case errors.Is(err, os.ErrNotExist):
				return nil
			default:
				return err
			}
			defer func() { _ = f.Close() }()

			read, err := io.ReadAll(f)
			if err == nil && !bytes.Equal(read, data) {
				err = errors.New("mismatch between Save and Load")
			}
			return err
		})
	}

	test.OK(t, g.Wait())
	saved := load(t, c, h)
	test.Equals(t, data, saved)
}
