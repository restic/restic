package khepri_test

import (
	"crypto/rand"
	"encoding/json"
	"flag"
	"io"
	mrand "math/rand"
	"sync"
	"testing"
	"time"

	"github.com/fd0/khepri"
)

const backendIDSize = 8

var maxWorkers = flag.Uint("workers", 100, "number of workers to test BlobList concurrent access against")

func randomID() []byte {
	buf := make([]byte, backendIDSize)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		panic(err)
	}
	return buf
}

func newBlob() khepri.Blob {
	return khepri.Blob{ID: randomID(), Size: uint64(mrand.Uint32())}
}

// Test basic functionality
func TestBlobList(t *testing.T) {
	bl := khepri.NewBlobList()

	b := newBlob()
	bl.Insert(b)

	for i := 0; i < 1000; i++ {
		bl.Insert(newBlob())
	}

	b2, err := bl.Find(khepri.Blob{ID: b.ID})
	ok(t, err)
	assert(t, b2.Compare(b) == 0, "items are not equal: want %v, got %v", b, b2)

	bl2 := khepri.NewBlobList()
	for i := 0; i < 1000; i++ {
		bl.Insert(newBlob())
	}

	b2, err = bl2.Find(b)
	assert(t, err != nil, "found ID in khepri that was never inserted: %v", b2)

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
func TestBlobListJSON(t *testing.T) {
	bl := khepri.NewBlobList()
	b := khepri.Blob{ID: []byte{1, 2, 3, 4}}
	bl.Insert(b)

	b2, err := bl.Find(b)
	ok(t, err)
	assert(t, b2.Compare(b) == 0, "items are not equal: want %v, got %v", b, b2)

	buf, err := json.Marshal(bl)
	ok(t, err)

	bl2 := khepri.BlobList{}
	json.Unmarshal(buf, &bl2)

	b2, err = bl2.Find(b)
	ok(t, err)
	assert(t, b2.Compare(b) == 0, "items are not equal: want %v, got %v", b, b2)

	buf, err = json.Marshal(bl2)
	ok(t, err)
}

// random insert/find access by several goroutines
func TestBlobListRandom(t *testing.T) {
	var wg sync.WaitGroup

	worker := func(bl *khepri.BlobList) {
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

		bl2 := khepri.NewBlobList()
		for i := 0; i < 200; i++ {
			bl2.Insert(newBlob())
		}

		bl2.Merge(bl)
	}

	bl := khepri.NewBlobList()

	for i := 0; uint(i) < *maxWorkers; i++ {
		wg.Add(1)
		go worker(bl)
	}

	wg.Wait()
}
