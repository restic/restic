package cache

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

func loadAndCompare(t testing.TB, be restic.Backend, h restic.Handle, data []byte) {
	buf, err := backend.LoadAll(context.TODO(), nil, be, h)
	if err != nil {
		t.Fatal(err)
	}

	if len(buf) != len(data) {
		t.Fatalf("wrong number of bytes read, want %v, got %v", len(data), len(buf))
	}

	if !bytes.Equal(buf, data) {
		t.Fatalf("wrong data returned, want:\n  %02x\ngot:\n  %02x", data[:16], buf[:16])
	}
}

func save(t testing.TB, be restic.Backend, h restic.Handle, data []byte) {
	err := be.Save(context.TODO(), h, restic.NewByteReader(data, be.Hasher()))
	if err != nil {
		t.Fatal(err)
	}
}

func remove(t testing.TB, be restic.Backend, h restic.Handle) {
	err := be.Remove(context.TODO(), h)
	if err != nil {
		t.Fatal(err)
	}
}

func randomData(n int) (restic.Handle, []byte) {
	data := test.Random(rand.Int(), n)
	id := restic.Hash(data)
	h := restic.Handle{
		Type: restic.IndexFile,
		Name: id.String(),
	}
	return h, data
}

func TestBackend(t *testing.T) {
	be := mem.New()
	c := TestNewCache(t)
	wbe := c.Wrap(be)

	h, data := randomData(5234142)

	// save directly in backend
	save(t, be, h, data)
	if c.Has(h) {
		t.Errorf("cache has file too early")
	}

	// load data via cache
	loadAndCompare(t, wbe, h, data)
	if !c.Has(h) {
		t.Errorf("cache doesn't have file after load")
	}

	// remove via cache
	remove(t, wbe, h)
	if c.Has(h) {
		t.Errorf("cache has file after remove")
	}

	// save via cache
	save(t, wbe, h, data)
	if !c.Has(h) {
		t.Errorf("cache doesn't have file after load")
	}

	// load data directly from backend
	loadAndCompare(t, be, h, data)

	// load data via cache
	loadAndCompare(t, be, h, data)

	// remove directly
	remove(t, be, h)
	if !c.Has(h) {
		t.Errorf("file not in cache any more")
	}

	// run stat
	_, err := wbe.Stat(context.TODO(), h)
	if err == nil {
		t.Errorf("expected error for removed file not found, got nil")
	}

	if !wbe.IsNotExist(err) {
		t.Errorf("Stat() returned error that does not match IsNotExist(): %v", err)
	}

	if c.Has(h) {
		t.Errorf("removed file still in cache after stat")
	}
}

type loadErrorBackend struct {
	restic.Backend
	loadError error
}

func (be loadErrorBackend) Load(ctx context.Context, h restic.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	time.Sleep(10 * time.Millisecond)
	return be.loadError
}

func TestErrorBackend(t *testing.T) {
	be := mem.New()
	c := TestNewCache(t)
	h, data := randomData(5234142)

	// save directly in backend
	save(t, be, h, data)

	testErr := errors.New("test error")
	errBackend := loadErrorBackend{
		Backend:   be,
		loadError: testErr,
	}

	loadTest := func(wg *sync.WaitGroup, be restic.Backend) {
		defer wg.Done()

		buf, err := backend.LoadAll(context.TODO(), nil, be, h)
		if err == testErr {
			return
		}

		if err != nil {
			t.Error(err)
			return
		}

		if !bytes.Equal(buf, data) {
			t.Errorf("data does not match")
		}
		time.Sleep(time.Millisecond)
	}

	wrappedBE := c.Wrap(errBackend)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go loadTest(&wg, wrappedBE)
	}

	wg.Wait()
}

func TestBackendRemoveBroken(t *testing.T) {
	be := mem.New()
	c := TestNewCache(t)

	h, data := randomData(5234142)
	// save directly in backend
	save(t, be, h, data)

	// prime cache with broken copy
	broken := append([]byte{}, data...)
	broken[0] ^= 0xff
	err := c.Save(h, bytes.NewReader(broken))
	test.OK(t, err)

	// loadall retries if broken data was returned
	buf, err := backend.LoadAll(context.TODO(), nil, c.Wrap(be), h)
	test.OK(t, err)

	if !bytes.Equal(buf, data) {
		t.Fatalf("wrong data returned")
	}

	// check that the cache now contains the correct data
	rd, err := c.load(h, 0, 0)
	defer func() {
		_ = rd.Close()
	}()
	test.OK(t, err)
	cached, err := io.ReadAll(rd)
	test.OK(t, err)
	if !bytes.Equal(cached, data) {
		t.Fatalf("wrong data cache")
	}
}
