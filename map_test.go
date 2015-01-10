package restic_test

import (
	"crypto/rand"
	"encoding/json"
	"flag"
	"io"
	mrand "math/rand"
	"sync"
	"testing"
	"time"

	"github.com/restic/restic"
	"github.com/restic/restic/backend"
)

var maxWorkers = flag.Uint("workers", 20, "number of workers to test Map concurrent access against")

func randomID() []byte {
	buf := make([]byte, backend.IDSize)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		panic(err)
	}
	return buf
}

func newBlob() restic.Blob {
	return restic.Blob{
		ID:          randomID(),
		Size:        uint64(mrand.Uint32()),
		Storage:     randomID(),
		StorageSize: uint64(mrand.Uint32()),
	}
}

// Test basic functionality
func TestMap(t *testing.T) {
	bl := restic.NewMap()

	b := newBlob()
	bl.Insert(b)

	for i := 0; i < 1000; i++ {
		bl.Insert(newBlob())
	}

	b2, err := bl.Find(restic.Blob{ID: b.ID, Size: b.Size})
	ok(t, err)
	assert(t, b2.Compare(b) == 0, "items are not equal: want %v, got %v", b, b2)

	b2, err = bl.FindID(b.ID)
	ok(t, err)
	assert(t, b2.Compare(b) == 0, "items are not equal: want %v, got %v", b, b2)

	bl2 := restic.NewMap()
	for i := 0; i < 1000; i++ {
		bl.Insert(newBlob())
	}

	b2, err = bl2.Find(b)
	assert(t, err != nil, "found ID in restic that was never inserted: %v", b2)

	bl2.Merge(bl)

	b2, err = bl2.Find(b)

	if err != nil {
		t.Fatal(err)
	}

	if b.Compare(b2) != 0 {
		t.Fatalf("items are not equal: want %v, got %v", b, b2)
	}
}

// Test JSON encode/decode
func TestMapJSON(t *testing.T) {
	bl := restic.NewMap()
	b := restic.Blob{ID: randomID()}
	bl.Insert(b)

	b2, err := bl.Find(b)
	ok(t, err)
	assert(t, b2.Compare(b) == 0, "items are not equal: want %v, got %v", b, b2)

	buf, err := json.Marshal(bl)
	ok(t, err)

	bl2 := restic.Map{}
	json.Unmarshal(buf, &bl2)

	b2, err = bl2.Find(b)
	ok(t, err)
	assert(t, b2.Compare(b) == 0, "items are not equal: want %v, got %v", b, b2)

	buf, err = json.Marshal(bl2)
	ok(t, err)
}

// random insert/find access by several goroutines
func TestMapRandom(t *testing.T) {
	var wg sync.WaitGroup

	worker := func(bl *restic.Map) {
		defer wg.Done()

		b := newBlob()
		bl.Insert(b)

		for i := 0; i < 200; i++ {
			bl.Insert(newBlob())
		}

		d := time.Duration(mrand.Intn(10)*100) * time.Millisecond
		time.Sleep(d)

		for i := 0; i < 100; i++ {
			b2, err := bl.Find(b)
			if err != nil {
				t.Fatal(err)
			}

			if b.Compare(b2) != 0 {
				t.Fatalf("items are not equal: want %v, got %v", b, b2)
			}
		}

		bl2 := restic.NewMap()
		for i := 0; i < 200; i++ {
			bl2.Insert(newBlob())
		}

		bl2.Merge(bl)
	}

	bl := restic.NewMap()

	for i := 0; uint(i) < *maxWorkers; i++ {
		wg.Add(1)
		go worker(bl)
	}

	wg.Wait()
}
