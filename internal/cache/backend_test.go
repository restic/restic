package cache

import (
	"bytes"
	"context"
	"math/rand"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/test"
)

func loadAndCompare(t testing.TB, be restic.Backend, h restic.Handle, data []byte) {
	buf, err := backend.LoadAll(context.TODO(), be, h)
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
	err := be.Save(context.TODO(), h, bytes.NewReader(data))
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
	copy(id[:], data)
	h := restic.Handle{
		Type: restic.IndexFile,
		Name: id.String(),
	}
	return h, data
}

func TestBackend(t *testing.T) {
	be := mem.New()

	c, cleanup := TestNewCache(t)
	defer cleanup()

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
		t.Errorf("cache dosen't have file after load")
	}

	// remove via cache
	remove(t, wbe, h)
	if c.Has(h) {
		t.Errorf("cache has file after remove")
	}

	// save via cache
	save(t, wbe, h, data)
	if !c.Has(h) {
		t.Errorf("cache dosen't have file after load")
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
