package cache

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/mem"
	backendtest "github.com/restic/restic/internal/backend/test"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

func loadAndCompare(t testing.TB, be backend.Backend, h backend.Handle, data []byte) {
	buf, err := backendtest.LoadAll(context.TODO(), be, h)
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

func save(t testing.TB, be backend.Backend, h backend.Handle, data []byte) {
	err := be.Save(context.TODO(), h, backend.NewByteReader(data, be.Hasher()))
	if err != nil {
		t.Fatal(err)
	}
}

func remove(t testing.TB, be backend.Backend, h backend.Handle) {
	err := be.Remove(context.TODO(), h)
	if err != nil {
		t.Fatal(err)
	}
}

func randomData(n int) (backend.Handle, []byte) {
	data := test.Random(rand.Int(), n)
	id := restic.Hash(data)
	h := backend.Handle{
		Type: backend.IndexFile,
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
	loadAndCompare(t, wbe, h, data)

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

type loadCountingBackend struct {
	backend.Backend
	ctr int
}

func (l *loadCountingBackend) Load(ctx context.Context, h backend.Handle, length int, offset int64, fn func(rd io.Reader) error) error {
	l.ctr++
	return l.Backend.Load(ctx, h, length, offset, fn)
}

func TestOutOfBoundsAccess(t *testing.T) {
	be := &loadCountingBackend{Backend: mem.New()}
	c := TestNewCache(t)
	wbe := c.Wrap(be)

	h, data := randomData(50)
	save(t, be, h, data)

	// load out of bounds
	err := wbe.Load(context.TODO(), h, 100, 100, func(rd io.Reader) error {
		t.Error("cache returned non-existent file section")
		return errors.New("broken")
	})
	test.Assert(t, strings.Contains(err.Error(), " is too short"), "expected too short error, got %v", err)
	test.Equals(t, 1, be.ctr, "expected file to be loaded only once")
	// file must nevertheless get cached
	if !c.Has(h) {
		t.Errorf("cache doesn't have file after load")
	}

	// start within bounds, but request too large chunk
	err = wbe.Load(context.TODO(), h, 100, 0, func(rd io.Reader) error {
		t.Error("cache returned non-existent file section")
		return errors.New("broken")
	})
	test.Assert(t, strings.Contains(err.Error(), " is too short"), "expected too short error, got %v", err)
	test.Equals(t, 1, be.ctr, "expected file to be loaded only once")
}

func TestForget(t *testing.T) {
	be := &loadCountingBackend{Backend: mem.New()}
	c := TestNewCache(t)
	wbe := c.Wrap(be)

	h, data := randomData(50)
	save(t, be, h, data)

	loadAndCompare(t, wbe, h, data)
	test.Equals(t, 1, be.ctr, "expected file to be loaded once")

	// must still exist even if load returns an error
	exp := errors.New("error")
	err := wbe.Load(context.TODO(), h, 0, 0, func(rd io.Reader) error {
		return exp
	})
	test.Equals(t, exp, err, "wrong error")
	test.Assert(t, c.Has(h), "missing cache entry")

	test.OK(t, c.Forget(h))
	test.Assert(t, !c.Has(h), "cache entry should have been removed")

	// cache it again
	loadAndCompare(t, wbe, h, data)
	test.Assert(t, c.Has(h), "missing cache entry")

	// forget must delete file only once
	err = c.Forget(h)
	test.Assert(t, strings.Contains(err.Error(), "circuit breaker prevents repeated deletion of cached file"), "wrong error message %q", err)
	test.Assert(t, c.Has(h), "cache entry should still exist")
}

type loadErrorBackend struct {
	backend.Backend
	loadError error
}

func (be loadErrorBackend) Load(_ context.Context, _ backend.Handle, _ int, _ int64, _ func(rd io.Reader) error) error {
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

	loadTest := func(wg *sync.WaitGroup, be backend.Backend) {
		defer wg.Done()

		buf, err := backendtest.LoadAll(context.TODO(), be, h)
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
