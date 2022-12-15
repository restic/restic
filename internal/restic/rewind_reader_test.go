package restic

import (
	"bytes"
	"crypto/md5"
	"hash"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/restic/restic/internal/test"
)

func TestByteReader(t *testing.T) {
	buf := []byte("foobar")
	for _, hasher := range []hash.Hash{nil, md5.New()} {
		fn := func() RewindReader {
			return NewByteReader(buf, hasher)
		}
		testRewindReader(t, fn, buf)
	}
}

func TestFileReader(t *testing.T) {
	buf := []byte("foobar")

	d := test.TempDir(t)
	filename := filepath.Join(d, "file-reader-test")
	err := os.WriteFile(filename, buf, 0600)
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		err := f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()

	for _, hasher := range []hash.Hash{nil, md5.New()} {
		fn := func() RewindReader {
			var hash []byte
			if hasher != nil {
				// must never fail according to interface
				_, err := hasher.Write(buf)
				if err != nil {
					panic(err)
				}
				hash = hasher.Sum(nil)
			}
			rd, err := NewFileReader(f, hash)
			if err != nil {
				t.Fatal(err)
			}
			return rd
		}

		testRewindReader(t, fn, buf)
	}
}

func testRewindReader(t *testing.T, fn func() RewindReader, data []byte) {
	seed := time.Now().UnixNano()
	t.Logf("seed is %d", seed)
	rnd := rand.New(rand.NewSource(seed))

	type ReaderTestFunc func(t testing.TB, r RewindReader, data []byte)
	var tests = []ReaderTestFunc{
		func(t testing.TB, rd RewindReader, data []byte) {
			if rd.Length() != int64(len(data)) {
				t.Fatalf("wrong length returned, want %d, got %d", int64(len(data)), rd.Length())
			}

			buf := make([]byte, len(data))
			_, err := io.ReadFull(rd, buf)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(buf, data) {
				t.Fatalf("wrong data returned")
			}

			if rd.Length() != int64(len(data)) {
				t.Fatalf("wrong length returned, want %d, got %d", int64(len(data)), rd.Length())
			}

			err = rd.Rewind()
			if err != nil {
				t.Fatal(err)
			}

			if rd.Length() != int64(len(data)) {
				t.Fatalf("wrong length returned, want %d, got %d", int64(len(data)), rd.Length())
			}

			buf2 := make([]byte, int64(len(data)))
			_, err = io.ReadFull(rd, buf2)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(buf2, data) {
				t.Fatalf("wrong data returned")
			}

			if rd.Length() != int64(len(data)) {
				t.Fatalf("wrong length returned, want %d, got %d", int64(len(data)), rd.Length())
			}

			if rd.Hash() != nil {
				hasher := md5.New()
				// must never fail according to interface
				_, _ = hasher.Write(buf2)
				if !bytes.Equal(rd.Hash(), hasher.Sum(nil)) {
					t.Fatal("hash does not match data")
				}
			}
		},
		func(t testing.TB, rd RewindReader, data []byte) {
			// read first bytes
			buf := make([]byte, rnd.Intn(len(data)))
			_, err := io.ReadFull(rd, buf)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(buf, data[:len(buf)]) {
				t.Fatalf("wrong data returned")
			}

			err = rd.Rewind()
			if err != nil {
				t.Fatal(err)
			}

			buf2 := make([]byte, rnd.Intn(len(data)))
			_, err = io.ReadFull(rd, buf2)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(buf2, data[:len(buf2)]) {
				t.Fatalf("wrong data returned")
			}

			// read remainder
			buf3 := make([]byte, len(data)-len(buf2))
			_, err = io.ReadFull(rd, buf3)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(buf3, data[len(buf2):]) {
				t.Fatalf("wrong data returned")
			}
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			rd := fn()
			test(t, rd, data)
		})
	}
}
