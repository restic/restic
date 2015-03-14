package restic

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/restic/restic/backend"
	"github.com/restic/restic/debug"
)

type Cache struct {
	base string
}

func NewCache(be backend.IDer) (c *Cache, err error) {
	// try to get explicit cache dir from environment
	dir := os.Getenv("RESTIC_CACHE")

	// otherwise try OS specific default
	if dir == "" {
		dir, err = GetCacheDir()
		if err != nil {
			return nil, err
		}
	}

	basedir := filepath.Join(dir, be.ID().String())
	debug.Log("Cache.New", "opened cache at %v", basedir)

	return &Cache{base: basedir}, nil
}

func (c *Cache) Has(t backend.Type, subtype string, id backend.ID) (bool, error) {
	// try to open file
	filename, err := c.filename(t, subtype, id)
	if err != nil {
		return false, err
	}

	fd, err := os.Open(filename)
	defer fd.Close()
	if err != nil {
		if os.IsNotExist(err) {
			debug.Log("Cache.Has", "test for file %v: not cached", filename)
			return false, nil
		}

		debug.Log("Cache.Has", "test for file %v: error %v", filename, err)
		return false, err
	}

	debug.Log("Cache.Has", "test for file %v: is cached", filename)
	return true, nil
}

func (c *Cache) Store(t backend.Type, subtype string, id backend.ID) (io.WriteCloser, error) {
	filename, err := c.filename(t, subtype, id)
	if err != nil {
		return nil, err
	}

	dirname := filepath.Dir(filename)
	err = os.MkdirAll(dirname, 0700)
	if err != nil {
		return nil, err
	}

	file, err := os.Create(filename)
	if err != nil {
		debug.Log("Cache.Store", "error creating file %v: %v", filename, err)
		return nil, err
	}

	debug.Log("Cache.Store", "created file %v", filename)
	return file, nil
}

func (c *Cache) Load(t backend.Type, subtype string, id backend.ID) (io.ReadCloser, error) {
	// try to open file
	filename, err := c.filename(t, subtype, id)
	if err != nil {
		return nil, err
	}

	return os.Open(filename)
}

func (c *Cache) Purge(t backend.Type, subtype string, id backend.ID) error {
	filename, err := c.filename(t, subtype, id)
	if err != nil {
		return err
	}

	err = os.Remove(filename)
	debug.Log("Cache.Purge", "Remove file %v: %v", filename, err)

	if err != nil && os.IsNotExist(err) {
		return nil
	}

	return err
}

func (c *Cache) Clear(s backend.Backend) error {
	list, err := c.List(backend.Snapshot)
	if err != nil {
		return err
	}

	for _, entry := range list {
		debug.Log("Cache.Clear", "found entry %v", entry)

		if ok, err := s.Test(backend.Snapshot, entry.ID); !ok || err != nil {
			debug.Log("Cache.Clear", "snapshot %v doesn't exist any more, removing %v", entry.ID, entry)

			err = c.Purge(backend.Snapshot, entry.Subtype, entry.ID)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

type CacheEntry struct {
	ID      backend.ID
	Subtype string
}

func (c CacheEntry) String() string {
	if c.Subtype != "" {
		return c.ID.Str() + "." + c.Subtype
	}
	return c.ID.Str()
}

func (c *Cache) List(t backend.Type) ([]CacheEntry, error) {
	var dir string

	switch t {
	case backend.Snapshot:
		dir = filepath.Join(c.base, "snapshots")
	case backend.Tree:
		dir = filepath.Join(c.base, "trees")
	default:
		return nil, fmt.Errorf("cache not supported for type %v", t)
	}

	fd, err := os.Open(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []CacheEntry{}, nil
		}
		return nil, err
	}
	defer fd.Close()

	fis, err := fd.Readdir(-1)
	if err != nil {
		return nil, err
	}

	entries := make([]CacheEntry, 0, len(fis))

	for _, fi := range fis {
		parts := strings.SplitN(fi.Name(), ".", 2)

		id, err := backend.ParseID(parts[0])
		// ignore invalid cache entries for now
		if err != nil {
			continue
		}

		e := CacheEntry{ID: id}

		if len(parts) == 2 {
			e.Subtype = parts[1]
		}

		entries = append(entries, e)
	}

	return entries, nil
}

// Construct file name for given Type.
func (c *Cache) filename(t backend.Type, subtype string, id backend.ID) (string, error) {
	filename := id.String()
	if subtype != "" {
		filename += "." + subtype
	}

	switch t {
	case backend.Snapshot:
		return filepath.Join(c.base, "snapshots", filename), nil
	case backend.Tree:
		return filepath.Join(c.base, "trees", filename), nil
	}

	return "", fmt.Errorf("cache not supported for type %v", t)
}

// high-level functions

// CacheSnapshotBlobs creates a cache of all the blobs used within the
// snapshot. It collects all blobs from all trees and saves the resulting map
// to the cache and returns the map.
func CacheSnapshotBlobs(s Server, c *Cache, id backend.ID) (*Map, error) {
	debug.Log("CacheSnapshotBlobs", "create cache for snapshot %v", id.Str())

	sn, err := LoadSnapshot(s, id)
	if err != nil {
		debug.Log("CacheSnapshotBlobs", "unable to load snapshot %v: %v", id.Str(), err)
		return nil, err
	}

	m := NewMap()

	// add top-level node
	m.Insert(sn.Tree)

	// start walker
	var wg sync.WaitGroup
	ch := make(chan WalkTreeJob)

	wg.Add(1)
	go func() {
		WalkTree(s, sn.Tree.Storage, nil, ch)
		wg.Done()
	}()

	for i := 0; i < maxConcurrencyPreload; i++ {
		wg.Add(1)
		go func() {
			for job := range ch {
				if job.Tree == nil {
					continue
				}
				debug.Log("CacheSnapshotBlobs", "got job %v", job)
				m.Merge(job.Tree.Map)
			}

			wg.Done()
		}()
	}

	wg.Wait()

	// save blob list for snapshot
	return m, c.StoreMap(id, m)
}

func (c *Cache) StoreMap(snid backend.ID, m *Map) error {
	wr, err := c.Store(backend.Snapshot, "blobs", snid)
	if err != nil {
		return nil
	}
	defer wr.Close()

	enc := json.NewEncoder(wr)
	err = enc.Encode(m)
	if err != nil {
		return err
	}

	return nil
}

func (c *Cache) LoadMap(s Server, snid backend.ID) (*Map, error) {
	rd, err := c.Load(backend.Snapshot, "blobs", snid)
	if err != nil {
		return nil, err
	}

	m := &Map{}
	err = json.NewDecoder(rd).Decode(m)
	return m, err
}
